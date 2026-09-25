package main

import (
	"context"
	"encoding/json"
	"sync/atomic"

	"github.com/Wei-Shaw/sub2api/pkg/pluginapi/sharedquota"
	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
)

const pluginID = "local.sub2api.shared-daily-quota"
const version = "0.1.2"

type runtime struct {
	pluginv1.UnimplementedTransportPluginServer
	config atomic.Pointer[sharedquota.Config]
}

func (*runtime) GetInfo(context.Context, *pluginv1.GetInfoRequest) (*pluginv1.GetInfoResponse, error) {
	return &pluginv1.GetInfoResponse{PluginId: pluginID, PluginVersion: version, ProtocolVersion: pluginv1.ProtocolVersion, TransportApiVersion: pluginv1.TransportAPIVersion, Capabilities: []string{sharedquota.Capability}}, nil
}
func (*runtime) Health(context.Context, *pluginv1.HealthRequest) (*pluginv1.HealthResponse, error) {
	return &pluginv1.HealthResponse{Healthy: true}, nil
}
func (*runtime) ValidateConfig(_ context.Context, req *pluginv1.ValidateConfigRequest) (*pluginv1.ValidateConfigResponse, error) {
	c, err := sharedquota.Parse(req.ConfigJson)
	if err != nil {
		return &pluginv1.ValidateConfigResponse{Message: err.Error()}, nil
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	return &pluginv1.ValidateConfigResponse{Valid: true, NormalizedConfigJson: raw}, nil
}
func (r *runtime) ApplyConfig(_ context.Context, req *pluginv1.ApplyConfigRequest) (*pluginv1.ApplyConfigResponse, error) {
	c, err := sharedquota.Parse(req.ConfigJson)
	if err != nil {
		return &pluginv1.ApplyConfigResponse{Message: err.Error()}, nil
	}
	r.config.Store(&c)
	return &pluginv1.ApplyConfigResponse{Applied: true}, nil
}
func (*runtime) TestConfig(_ context.Context, req *pluginv1.TestConfigRequest) (*pluginv1.TestConfigResponse, error) {
	_, err := sharedquota.Parse(req.ConfigJson)
	if err != nil {
		return &pluginv1.TestConfigResponse{Message: err.Error()}, nil
	}
	return &pluginv1.TestConfigResponse{Success: true, Message: "配置有效"}, nil
}
func main() { pluginv1.Serve(&runtime{}) }
