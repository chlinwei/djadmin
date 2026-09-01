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
)
