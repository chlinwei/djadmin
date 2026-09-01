package sysconfig

import (
	"net/http"

	"autoadmin/internal/shared/apperror"
)

var (
	ErrNotFound        = apperror.New(apperror.CodeNotFound, "参数不存在")
	ErrReadonly        = apperror.New(apperror.CodeInvalidArgument, "只读参数不可修改")
	ErrNoDefault       = apperror.New(apperror.CodeInvalidArgument, "参数没有默认值")
	ErrValueNotInteger = apperror.New(apperror.CodeInvalidArgument, "参数值必须是整数")
	ErrValueNotBoolean = apperror.New(apperror.CodeInvalidArgument, "参数值必须是布尔值")
	ErrValueNotJSON    = apperror.New(apperror.CodeInvalidArgument, "参数值必须是有效 JSON")

	ErrQueryInternal = apperror.NewWithHTTP(apperror.CodeInternal, "查询参数失败", http.StatusInternalServerError)
)
