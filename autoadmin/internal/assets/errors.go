package assets

import "autoadmin/internal/shared/apperror"

var (
	ErrNotFound         = apperror.New(apperror.CodeNotFound, "资产不存在")
	ErrInvalid          = apperror.New(apperror.CodeInvalidArgument, "资产参数无效")
	ErrDuplicate        = apperror.New(apperror.CodeInvalidArgument, "名称或编码已存在")
	ErrInvalidRelation  = apperror.New(apperror.CodeInvalidArgument, "关联资产不存在")
	ErrDeleteProtected  = apperror.New(apperror.CodeInvalidArgument, "资产仍被其他记录引用，无法删除")
	ErrGroupCycle       = apperror.New(apperror.CodeInvalidArgument, "主机分组不能形成循环层级")
	ErrQueryInternal    = apperror.New(apperror.CodeInternal, "查询资产失败")
	ErrAgentUnavailable = apperror.New(apperror.CodeInvalidArgument, "部署主机 Agent 数据面尚未连接，无法执行应用控制")
	// 主机的双重唯一标识：instance_name（业务标识）和 ip（寻址标识）均须全局唯一。
	// 主机不再有 agent_id——dj-agent 的全局标识就是 instance_name。
	ErrHostIPRequired            = apperror.New(apperror.CodeInvalidArgument, "IP 必填：IP 是主机的寻址标识")
	ErrHostIPDuplicate           = apperror.New(apperror.CodeInvalidArgument, "该 IP 已被其他主机使用，主机的寻址标识是 IP")
	ErrHostInstanceNameRequired  = apperror.New(apperror.CodeInvalidArgument, "实例名必填：实例名是主机的业务标识，也是 dj-agent 的 DJ_AGENT_INSTANCE_NAME")
	ErrHostInstanceNameDuplicate = apperror.New(apperror.CodeInvalidArgument, "该实例名已被其他主机使用，主机的业务标识是实例名")
)
