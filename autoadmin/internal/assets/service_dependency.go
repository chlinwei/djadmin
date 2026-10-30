package assets

import (
	"context"
	"database/sql"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/apperror"
	"autoadmin/internal/shared/pagination"
)

var (
	ErrDependencyScope        = apperror.New(apperror.CodeInvalidArgument, "依赖两端必须属于同一项目、同一环境，且项目与环境不能为空")
	ErrDependencyTypeDisabled = apperror.New(apperror.CodeInvalidArgument, "交互类型已停用，不能用于新增或切换依赖类型")
	ErrServiceHasDependencies = apperror.New(apperror.CodeInvalidArgument, "服务存在依赖或被依赖关系，请先解除关系再删除")
	ErrDependencyDuplicate    = apperror.New(apperror.CodeInvalidArgument, "同方向、同交互类型和功能范围的依赖关系已存在")
	dependencyTypeCode        = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
)

type DependencyTypeInput struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Enabled     *bool  `json:"enabled"`
	Description string `json:"description"`
}

type DependencyDetailsInput struct {
	InteractionTypeID int64  `json:"interaction_type_id"`
	Strength          string `json:"strength"`
	Protocol          string `json:"protocol"`
	FunctionScope     string `json:"function_scope"`
	FailureImpact     string `json:"failure_impact"`
	FallbackPlan      string `json:"fallback_plan"`
}

type ServiceDependencyInput struct {
	SourceServiceID int64 `json:"source_service_id"`
	TargetServiceID int64 `json:"target_service_id"`
	DependencyDetailsInput
	// Reverse is an explicit second edge with its own interaction and business semantics.
	Reverse *DependencyDetailsInput `json:"reverse,omitempty"`
}

type DependencyFilter struct {
	ProjectID, EnvironmentID, BusinessID, ServiceID     int64
	SourceServiceID, TargetServiceID, InteractionTypeID int64
	Strength, Search                                    string
}

func validDependencyStrength(value string) bool {
	return value == "strong" || value == "weak" || value == "unknown"
}

func normalizeDependencyDetails(input *DependencyDetailsInput) error {
	input.Strength = strings.TrimSpace(input.Strength)
	if input.Strength == "" {
		input.Strength = "unknown"
	}
	input.Protocol = strings.TrimSpace(input.Protocol)
	input.FunctionScope = strings.TrimSpace(input.FunctionScope)
	input.FailureImpact = strings.TrimSpace(input.FailureImpact)
	input.FallbackPlan = strings.TrimSpace(input.FallbackPlan)
	if input.InteractionTypeID < 1 || !validDependencyStrength(input.Strength) || input.FunctionScope == "" ||
		utf8.RuneCountInString(input.Protocol) > 128 || utf8.RuneCountInString(input.FunctionScope) > 120 ||
		utf8.RuneCountInString(input.FailureImpact) > 500 || utf8.RuneCountInString(input.FallbackPlan) > 500 {
		return ErrInvalid
	}
	return nil
}

func dependencyID(value int64) sql.NullInt64 {
	return sql.NullInt64{Int64: value, Valid: value > 0}
}

func (f DependencyFilter) params(p pagination.Page) db.ListServiceDependenciesParams {
	return db.ListServiceDependenciesParams{
		ProjectID: dependencyID(f.ProjectID), EnvironmentID: dependencyID(f.EnvironmentID),
		BusinessID: dependencyID(f.BusinessID), ServiceID: dependencyID(f.ServiceID),
		SourceServiceID: dependencyID(f.SourceServiceID), TargetServiceID: dependencyID(f.TargetServiceID),
		InteractionTypeID: dependencyID(f.InteractionTypeID), Strength: sql.NullString{String: f.Strength, Valid: f.Strength != ""},
		Pattern: pattern(f.Search).String, Limit: p.Size, Offset: p.Offset,
	}
}

func (s *Service) ListServiceDependencies(ctx context.Context, f DependencyFilter, p pagination.Page) ([]db.ListServiceDependenciesRow, int64, error) {
	v := f.params(p)
	count, err := s.repository.queries.CountServiceDependencies(ctx, db.CountServiceDependenciesParams{
		ProjectID: v.ProjectID, EnvironmentID: v.EnvironmentID, BusinessID: v.BusinessID, ServiceID: v.ServiceID,
		SourceServiceID: v.SourceServiceID, TargetServiceID: v.TargetServiceID, InteractionTypeID: v.InteractionTypeID,
		Strength: v.Strength, Pattern: v.Pattern,
	})
	if err != nil {
		return nil, 0, translate(err)
	}
	rows, err := s.repository.queries.ListServiceDependencies(ctx, v)
	return rows, count, translate(err)
}

func (s *Service) GetServiceDependency(ctx context.Context, id int64) (db.GetServiceDependencyRow, error) {
	row, err := s.repository.queries.GetServiceDependency(ctx, id)
	return row, translate(err)
}

func (s *Service) GetServiceDependencyType(ctx context.Context, id int64) (db.AssetsServiceDependencyType, error) {
	row, err := s.repository.queries.GetServiceDependencyType(ctx, id)
	return row, translate(err)
}

func (s *Service) SaveServiceDependency(ctx context.Context, id int64, input ServiceDependencyInput) ([]db.GetServiceDependencyRow, error) {
	if input.SourceServiceID < 1 || input.TargetServiceID < 1 || input.SourceServiceID == input.TargetServiceID || (id > 0 && input.Reverse != nil) {
		return nil, ErrInvalid
	}
	if err := normalizeDependencyDetails(&input.DependencyDetailsInput); err != nil {
		return nil, err
	}
	if input.Reverse != nil {
		if err := normalizeDependencyDetails(input.Reverse); err != nil {
			return nil, err
		}
	}
	rows, err := s.repository.saveServiceDependency(ctx, id, input)
	return rows, translate(err)
}

func (r *Repository) saveServiceDependency(ctx context.Context, id int64, input ServiceDependencyInput) ([]db.GetServiceDependencyRow, error) {
	tx, err := r.pool.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	q := db.New(tx)
	var old db.AssetsServiceDependency
	if id > 0 {
		old, err = q.LockServiceDependency(ctx, id)
		if err != nil {
			return nil, err
		}
	}
	typeIDs := []int64{input.InteractionTypeID}
	if input.Reverse != nil && input.Reverse.InteractionTypeID != input.InteractionTypeID {
		typeIDs = append(typeIDs, input.Reverse.InteractionTypeID)
	}
	sort.Slice(typeIDs, func(i, j int) bool { return typeIDs[i] < typeIDs[j] })
	for _, typeID := range typeIDs {
		kind, err := q.LockServiceDependencyType(ctx, typeID)
		if err != nil {
			return nil, err
		}
		// A disabled type remains editable on its existing edge, but cannot acquire new edges.
		if !kind.Enabled && !(id > 0 && old.InteractionTypeID == typeID) {
			return nil, ErrDependencyTypeDisabled
		}
	}
	serviceIDs := []int64{input.SourceServiceID, input.TargetServiceID}
	sort.Slice(serviceIDs, func(i, j int) bool { return serviceIDs[i] < serviceIDs[j] })
	contexts := make(map[int64]db.GetServiceDependencyContextRow, 2)
	// Share endpoint locks with service deletion before checking its tombstone.
	// This prevents a new dependency from appearing after external log cleanup has started.
	for _, serviceID := range serviceIDs {
		item, err := q.GetServiceDependencyContext(ctx, serviceID)
		if err != nil {
			return nil, err
		}
		if _, err := q.LockServiceDependencyDeletionTombstone(ctx, serviceID); err == nil {
			return nil, ErrServiceLogIdentityDeleting
		} else if err != sql.ErrNoRows {
			return nil, err
		}
		contexts[serviceID] = item
	}
	source, target := contexts[input.SourceServiceID], contexts[input.TargetServiceID]
	if !sameDependencyScope(source, target) {
		return nil, ErrDependencyScope
	}
	now := time.Now().UTC()
	write := func(source, target db.GetServiceDependencyContextRow, details DependencyDetailsInput, edgeID int64) (int64, error) {
		if edgeID > 0 {
			err := q.UpdateServiceDependency(ctx, db.UpdateServiceDependencyParams{
				ID: edgeID, UpdateTime: now, SourceServiceID: source.ID, TargetServiceID: target.ID,
				SourceBusinessID: source.BusinessSystemID, TargetBusinessID: target.BusinessSystemID,
				ProjectID: source.ProjectID.Int64, EnvironmentID: source.EnvironmentID.Int64,
				InteractionTypeID: details.InteractionTypeID, Strength: details.Strength, Protocol: details.Protocol,
				FunctionScope: details.FunctionScope, FailureImpact: details.FailureImpact, FallbackPlan: details.FallbackPlan,
			})
			return edgeID, err
		}
		return q.CreateServiceDependency(ctx, db.CreateServiceDependencyParams{
			CreateTime: now, UpdateTime: now, SourceServiceID: source.ID, TargetServiceID: target.ID,
			SourceBusinessID: source.BusinessSystemID, TargetBusinessID: target.BusinessSystemID,
			ProjectID: source.ProjectID.Int64, EnvironmentID: source.EnvironmentID.Int64,
			InteractionTypeID: details.InteractionTypeID, Strength: details.Strength, Protocol: details.Protocol,
			FunctionScope: details.FunctionScope, FailureImpact: details.FailureImpact, FallbackPlan: details.FallbackPlan,
		})
	}
	savedID, err := write(source, target, input.DependencyDetailsInput, id)
	if err != nil {
		return nil, err
	}
	ids := []int64{savedID}
	if input.Reverse != nil {
		reverseID, err := write(target, source, *input.Reverse, 0)
		if err != nil {
			return nil, err
		}
		ids = append(ids, reverseID)
	}
	rows := make([]db.GetServiceDependencyRow, 0, len(ids))
	for _, savedID := range ids {
		item, err := q.GetServiceDependency(ctx, savedID)
		if err != nil {
			return nil, err
		}
		rows = append(rows, item)
	}
	return rows, tx.Commit()
}

func sameDependencyScope(a, b db.GetServiceDependencyContextRow) bool {
	return a.ProjectID.Valid && b.ProjectID.Valid && a.ProjectID.Int64 > 0 && a.ProjectID.Int64 == b.ProjectID.Int64 &&
		a.EnvironmentID.Valid && b.EnvironmentID.Valid && a.EnvironmentID.Int64 > 0 && a.EnvironmentID.Int64 == b.EnvironmentID.Int64
}

func (s *Service) DeleteServiceDependency(ctx context.Context, id int64) error {
	tx, err := s.repository.pool.BeginTx(ctx, nil)
	if err != nil {
		return translate(err)
	}
	defer tx.Rollback()
	q := db.New(tx)
	if _, err = q.LockServiceDependency(ctx, id); err != nil {
		return translate(err)
	}
	if err = q.DeleteServiceDependency(ctx, id); err != nil {
		return translate(err)
	}
	return translate(tx.Commit())
}

func (s *Service) ListServiceDependencyTypes(ctx context.Context, search string, enabled *bool, p pagination.Page) ([]db.AssetsServiceDependencyType, int64, error) {
	flag := sql.NullBool{}
	if enabled != nil {
		flag = sql.NullBool{Bool: *enabled, Valid: true}
	}
	count, err := s.repository.queries.CountServiceDependencyTypes(ctx, db.CountServiceDependencyTypesParams{Pattern: pattern(search).String, Enabled: flag})
	if err != nil {
		return nil, 0, translate(err)
	}
	rows, err := s.repository.queries.ListServiceDependencyTypes(ctx, db.ListServiceDependencyTypesParams{Pattern: pattern(search).String, Enabled: flag, Limit: p.Size, Offset: p.Offset})
	return rows, count, translate(err)
}

func (s *Service) SaveServiceDependencyType(ctx context.Context, id int64, input DependencyTypeInput) (db.AssetsServiceDependencyType, error) {
	input.Code = strings.TrimSpace(input.Code)
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if !dependencyTypeCode.MatchString(input.Code) || input.Name == "" || utf8.RuneCountInString(input.Name) > 128 || utf8.RuneCountInString(input.Description) > 500 {
		return db.AssetsServiceDependencyType{}, ErrInvalid
	}
	tx, err := s.repository.pool.BeginTx(ctx, nil)
	if err != nil {
		return db.AssetsServiceDependencyType{}, translate(err)
	}
	defer tx.Rollback()
	q := db.New(tx)
	enabled := boolValue(input.Enabled, true)
	if id > 0 {
		old, err := q.LockServiceDependencyType(ctx, id)
		if err != nil {
			return db.AssetsServiceDependencyType{}, translate(err)
		}
		if old.Code != input.Code {
			return db.AssetsServiceDependencyType{}, apperror.New(apperror.CodeInvalidArgument, "交互类型编码创建后不可修改")
		}
		if input.Enabled == nil {
			enabled = old.Enabled
		}
	}
	now := time.Now().UTC()
	if id == 0 {
		id, err = q.CreateServiceDependencyType(ctx, db.CreateServiceDependencyTypeParams{CreateTime: now, UpdateTime: now, Code: input.Code, Name: input.Name, Enabled: enabled, Description: input.Description})
	} else {
		err = q.UpdateServiceDependencyType(ctx, db.UpdateServiceDependencyTypeParams{ID: id, UpdateTime: now, Code: input.Code, Name: input.Name, Enabled: enabled, Description: input.Description})
	}
	if err != nil {
		return db.AssetsServiceDependencyType{}, translate(err)
	}
	item, err := q.GetServiceDependencyType(ctx, id)
	if err != nil {
		return item, translate(err)
	}
	return item, translate(tx.Commit())
}

func (s *Service) DeleteServiceDependencyType(ctx context.Context, id int64) error {
	tx, err := s.repository.pool.BeginTx(ctx, nil)
	if err != nil {
		return translate(err)
	}
	defer tx.Rollback()
	q := db.New(tx)
	if _, err = q.LockServiceDependencyType(ctx, id); err != nil {
		return translate(err)
	}
	count, err := q.CountServiceDependenciesByType(ctx, id)
	if err != nil {
		return translate(err)
	}
	if count > 0 {
		return apperror.New(apperror.CodeInvalidArgument, "交互类型仍被依赖关系引用，不能删除；可停用")
	}
	if err = q.DeleteServiceDependencyType(ctx, id); err != nil {
		return translate(err)
	}
	return translate(tx.Commit())
}
