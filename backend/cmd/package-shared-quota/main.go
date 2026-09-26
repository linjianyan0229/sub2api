// 本地构建并签名共享额度插件；签名私钥由调用者保存在仓库外。
package main

import (
	"archive/zip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	goExe := flag.String("go", "go", "Go 编译器路径")
	output := flag.String("output", "", "仓库外的输出目录")
	keyPath := flag.String("key", "", "仓库外的 Ed25519 签名私钥文件")
	flag.Parse()
	if *output == "" || *keyPath == "" {
		return fmt.Errorf("必须提供 -output 和 -key")
	}
	if err := os.MkdirAll(*output, 0700); err != nil {
		return err
	}
	key, err := os.ReadFile(*keyPath)
	if os.IsNotExist(err) {
		_, private, e := ed25519.GenerateKey(rand.Reader)
		if e != nil {
			return e
		}
		key = private
		if e = os.WriteFile(*keyPath, key, 0600); e != nil {
			return e
		}
	} else if err != nil {
		return err
	}
	if len(key) != ed25519.PrivateKeySize {
		return fmt.Errorf("签名私钥长度无效")
	}
	private := ed25519.PrivateKey(key)
	files := map[string][]byte{}
	runtimes := map[string]map[string]string{}
	staging, err := os.MkdirTemp(*output, "shared-quota-build-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(staging) }()
	for _, target := range []struct{ os, arch string }{{"linux", "amd64"}, {"linux", "arm64"}, {"windows", "amd64"}} {
		name := target.os + "-" + target.arch
		path := "runtimes/" + name + "/plugin"
		if target.os == "windows" {
			path += ".exe"
		}
		binary := filepath.Join(staging, name)
		cmd := exec.Command(*goExe, "build", "-trimpath", "-ldflags=-s -w", "-o", binary, "./cmd/shared-quota-plugin")
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+target.os, "GOARCH="+target.arch)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
		body, err := os.ReadFile(binary)
		if err != nil {
			return err
		}
		files[path] = body
		runtimes[name] = map[string]string{"path": path}
	}
	for _, name := range []string{"index.html", "style.css", "app.js"} {
		body, err := os.ReadFile(filepath.Join("..", "plugins", "shared-daily-quota", "ui", name))
		if err != nil {
			return err
		}
		files["ui/"+name] = body
	}
	hashes := map[string]string{}
	for path, body := range files {
		sum := sha256.Sum256(body)
		hashes[path] = hex.EncodeToString(sum[:])
	}
	manifest := map[string]any{
		"schema_version": 1, "id": "local.sub2api.shared-daily-quota", "name": "全站每日共享额度", "version": "0.1.3", "author": "Local",
		"requires":     map[string]any{"sub2api": ">=0.2.9 <0.3.0", "recommended_sub2api_version": "0.2.9", "tested_sub2api_versions": []string{"0.2.9"}, "plugin_protocol": 1, "transport_api": 1, "ui_bridge": 1},
		"capabilities": []map[string]string{{"id": "billing.shared_daily_quota.v1", "platform": "all", "account_type": "all"}},
		"runtimes":     runtimes, "ui": map[string]string{"entrypoint": "ui/index.html"}, "files": hashes,
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	files["manifest.json"] = raw
	signature, err := json.Marshal(map[string]string{"algorithm": "ed25519", "key_id": "shared-daily-quota-local", "signature": base64.StdEncoding.EncodeToString(ed25519.Sign(private, raw))})
	if err != nil {
		return err
	}
	files["signature.json"] = signature
	archive := filepath.Join(*output, "shared-daily-quota-0.1.3.s2plugin")
	f, err := os.Create(archive)
	if err != nil {
		return err
	}
	z := zip.NewWriter(f)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		w, e := z.Create(name)
		if e != nil {
			_ = f.Close()
			return e
		}
		if _, e = w.Write(files[name]); e != nil {
			_ = f.Close()
			return e
		}
	}
	if err = z.Close(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	public := base64.StdEncoding.EncodeToString(private.Public().(ed25519.PublicKey))
	trust := fmt.Sprintf("plugins:\n  trusted_publishers:\n    shared-daily-quota-local: %q\n", public)
	if err := os.WriteFile(filepath.Join(*output, "trusted-publisher.yaml"), []byte(trust), 0644); err != nil {
		return err
	}
	fmt.Println(archive)
	return nil
}
