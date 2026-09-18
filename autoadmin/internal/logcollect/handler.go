// Package logcollect 承载日志采集与日志存储两条链路：
//
//   - 采集：Filebeat 纳管目标的离线安装/卸载与启停、采集配置渲染与下发、链路体检、数据流清理；
//   - 存储：Elasticsearch 集群配置、索引模板与 ILM、ingest pipeline、日志检索与存储水位。
//
// 分层约定：渲染层（log_config_render.go）是纯函数、不访问数据库，便于单测；
// 数据流是服务级命名（见 internal/shared/logstream），档位编码在流名尾部供 ILM 后缀匹配。
package logcollect

import (
	"context"
	"database/sql"

	"autoadmin/internal/agent"
	"autoadmin/internal/assets"
	"autoadmin/internal/automation"
)

type Handler struct {
	db          *sql.DB
	gateway     *agent.Gateway
	jobs        *automation.Handler
	secrets     *assets.SecretEncryptor
	packageRoot string
	// 目标安装缺少主机平台/架构信息时主动补采一次资产信息（由 assets 域注入，
	// 避免日志采集反向依赖资产采集实现）。
	refreshHostInfo func(ctx context.Context, hostID int64) error
}

// NewHandler 装配日志采集域 Handler。凭据加解密器与软件包根目录由调用方（router）统一构造后注入：
// 部署模板/ES 集群的口令加解密、以及 Filebeat 离线包的读取都依赖这两者，
// 集中构造保证与监控域指向同一份配置和同一个 media 目录。
func NewHandler(db *sql.DB, gateway *agent.Gateway, jobs *automation.Handler, secrets *assets.SecretEncryptor, packageRoot string) *Handler {
	return &Handler{db: db, gateway: gateway, jobs: jobs, secrets: secrets, packageRoot: packageRoot}
}

// SetHostInfoRefresher 注入主机资产补采能力（assets.Handler.RefreshHostInfoByID）。
func (handler *Handler) SetHostInfoRefresher(refresher func(ctx context.Context, hostID int64) error) {
	handler.refreshHostInfo = refresher
}
