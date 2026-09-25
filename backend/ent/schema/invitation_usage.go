package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// InvitationUsage 不关联级联删除：删码或删用户不能释放已成功注册的 IP。
type InvitationUsage struct{ ent.Schema }

func (InvitationUsage) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "invitation_usages"}}
}

func (InvitationUsage) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("redeem_code_id"),
		field.Int64("user_id"),
		field.String("username").Default(""),
		field.String("email").Default(""),
		// 旧记录缺少注册 IP，保留 NULL，不能拿登录 IP 回填。
		field.String("ip").MaxLen(45).Optional().Nillable().Unique(),
		field.Time("created_at").Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (InvitationUsage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("redeem_code_id", "user_id").Unique(),
		index.Fields("redeem_code_id", "id"),
	}
}
