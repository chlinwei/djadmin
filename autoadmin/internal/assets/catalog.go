package assets

import (
	"context"
	"database/sql"
	"strings"
	"time"

	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/pagination"
)

type Application struct {
	ID                      int64                `json:"id"`
	CreateTime              string               `json:"create_time"`
	UpdateTime              string               `json:"update_time"`
	Remark                  *string              `json:"remark"`
	Name                    string               `json:"name"`
	Code                    string               `json:"code"`
	Category                string               `json:"category"`
	Vendor                  string               `json:"vendor"`
	Description             string               `json:"description"`
	Enabled                 bool                 `json:"enabled"`
	Versions                []ApplicationVersion `json:"versions"`
	VersionCount            int64                `json:"version_count"`
	DeploymentTemplateCount int32                `json:"deployment_template_count"`
	DeploymentCount         int32                `json:"deployment_count"`
}
type ApplicationInput struct {
	Name        string  `json:"name"`
	Code        string  `json:"code"`
	Category    string  `json:"category"`
	Vendor      string  `json:"vendor"`
	Description string  `json:"description"`
	Enabled     *bool   `json:"enabled"`
	Remark      *string `json:"remark"`
}
type ApplicationVersion struct {
	ID              int64   `json:"id"`
	CreateTime      string  `json:"create_time"`
	UpdateTime      string  `json:"update_time"`
	Remark          *string `json:"remark"`
	Application     int64   `json:"application"`
	ApplicationName string  `json:"application_name"`
	Version         string  `json:"version"`
	ReleaseDate     *string `json:"release_date"`
	EndOfSupport    *string `json:"end_of_support"`
	Enabled         bool    `json:"enabled"`
}
type ApplicationVersionInput struct {
	Application  int64   `json:"application"`
	Version      string  `json:"version"`
	ReleaseDate  *string `json:"release_date"`
	EndOfSupport *string `json:"end_of_support"`
	Enabled      *bool   `json:"enabled"`
	Remark       *string `json:"remark"`
}
type ClusterProfile struct {
	ID              int64   `json:"id"`
	CreateTime      string  `json:"create_time"`
	UpdateTime      string  `json:"update_time"`
	Remark          *string `json:"remark"`
	Name            string  `json:"name"`
	Code            string  `json:"code"`
	ProfileType     string  `json:"profile_type"`
	ClusterType     string  `json:"cluster_type"`
	Enabled         bool    `json:"enabled"`
	Application     *int64  `json:"application"`
	ApplicationName string  `json:"application_name"`
	ServiceCount    int32   `json:"service_count"`
}
type ClusterProfileInput struct {
	Name        string  `json:"name"`
	Code        string  `json:"code"`
	ProfileType string  `json:"profile_type"`
	ClusterType string  `json:"cluster_type"`
	Enabled     *bool   `json:"enabled"`
	Application *int64  `json:"application"`
	Remark      *string `json:"remark"`
}

func (r *Repository) ListApplications(ctx context.Context, search string, page pagination.Page) ([]db.ListApplicationsRow, int64, error) {
	p := pattern(search)
	count, err := r.queries.CountApplications(ctx, db.CountApplicationsParams{Pattern: p})
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.queries.ListApplications(ctx, db.ListApplicationsParams{Pattern: p, Limit: page.Size, Offset: page.Offset})
	return rows, count, err
}
func (r *Repository) GetApplication(ctx context.Context, id int64) (db.GetApplicationRow, error) {
	return r.queries.GetApplication(ctx, id)
}
func (r *Repository) CreateApplication(ctx context.Context, p db.CreateApplicationParams) (int64, error) {
	result, err := r.queries.CreateApplication(ctx, p)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
func (r *Repository) UpdateApplication(ctx context.Context, p db.UpdateApplicationParams) error {
	return r.queries.UpdateApplication(ctx, p)
}
func (r *Repository) DeleteApplication(ctx context.Context, id int64) error {
	return r.queries.DeleteApplication(ctx, id)
}
func (r *Repository) ListVersions(ctx context.Context, applicationID int64, search string, page pagination.Page) ([]db.ListApplicationVersionsRow, int64, error) {
	p := pattern(search)
	count, err := r.queries.CountApplicationVersions(ctx, db.CountApplicationVersionsParams{ApplicationID: applicationID, Pattern: p})
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.queries.ListApplicationVersions(ctx, db.ListApplicationVersionsParams{ApplicationID: applicationID, Pattern: p, Limit: page.Size, Offset: page.Offset})
	return rows, count, err
}
func (r *Repository) GetVersion(ctx context.Context, id int64) (db.GetApplicationVersionRow, error) {
	return r.queries.GetApplicationVersion(ctx, id)
}
func (r *Repository) CreateVersion(ctx context.Context, p db.CreateApplicationVersionParams) (int64, error) {
	result, err := r.queries.CreateApplicationVersion(ctx, p)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
func (r *Repository) UpdateVersion(ctx context.Context, p db.UpdateApplicationVersionParams) error {
	return r.queries.UpdateApplicationVersion(ctx, p)
}
func (r *Repository) DeleteVersion(ctx context.Context, id int64) error {
	return r.queries.DeleteApplicationVersion(ctx, id)
}
func (r *Repository) ListProfiles(ctx context.Context, applicationID int64, search string, page pagination.Page) ([]db.ListClusterProfilesRow, int64, error) {
	p := pattern(search)
	app := sql.NullInt64{Int64: applicationID, Valid: applicationID > 0}
	count, err := r.queries.CountClusterProfiles(ctx, db.CountClusterProfilesParams{ApplicationID: app, Pattern: p})
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.queries.ListClusterProfiles(ctx, db.ListClusterProfilesParams{ApplicationID: app, Pattern: p, Limit: page.Size, Offset: page.Offset})
	return rows, count, err
}
func (r *Repository) GetProfile(ctx context.Context, id int64) (db.GetClusterProfileRow, error) {
	return r.queries.GetClusterProfile(ctx, id)
}
func (r *Repository) CreateProfile(ctx context.Context, p db.CreateClusterProfileParams) (int64, error) {
	result, err := r.queries.CreateClusterProfile(ctx, p)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
func (r *Repository) UpdateProfile(ctx context.Context, p db.UpdateClusterProfileParams) error {
	return r.queries.UpdateClusterProfile(ctx, p)
}
func (r *Repository) DeleteProfile(ctx context.Context, id int64) error {
	return r.queries.DeleteClusterProfile(ctx, id)
}

func (s *Service) ListApplications(ctx context.Context, search string, page pagination.Page) ([]Application, int64, error) {
	rows, count, err := s.repository.ListApplications(ctx, search, page)
	result := make([]Application, 0, len(rows))
	for _, row := range rows {
		result = append(result, applicationList(row))
	}
	return result, count, translate(err)
}
func (s *Service) GetApplication(ctx context.Context, id int64) (Application, error) {
	row, err := s.repository.GetApplication(ctx, id)
	return applicationDetail(row), translate(err)
}
func (s *Service) CreateApplication(ctx context.Context, input ApplicationInput) (Application, error) {
	if !validNamed(input.Name, input.Code) {
		return Application{}, ErrInvalid
	}
	now := time.Now().UTC()
	id, err := s.repository.CreateApplication(ctx, db.CreateApplicationParams{CreateTime: now, UpdateTime: now, Remark: nullString(input.Remark), Name: strings.TrimSpace(input.Name), Category: input.Category, Code: strings.TrimSpace(input.Code), Description: input.Description, Enabled: enabled(input.Enabled), Vendor: input.Vendor})
	if err != nil {
		return Application{}, translate(err)
	}
	return s.GetApplication(ctx, id)
}
func (s *Service) UpdateApplication(ctx context.Context, id int64, input ApplicationInput) (Application, error) {
	current, err := s.repository.GetApplication(ctx, id)
	if err != nil {
		return Application{}, translate(err)
	}
	if input.Name == "" {
		input.Name = current.Name
	}
	if input.Code == "" {
		input.Code = current.Code
	}
	if input.Category == "" {
		input.Category = current.Category
	}
	if input.Vendor == "" {
		input.Vendor = current.Vendor
	}
	if input.Description == "" {
		input.Description = current.Description
	}
	if input.Remark == nil {
		input.Remark = stringValue(current.Remark)
	}
	value := current.Enabled
	if input.Enabled != nil {
		value = *input.Enabled
	}
	err = s.repository.UpdateApplication(ctx, db.UpdateApplicationParams{UpdateTime: time.Now().UTC(), Remark: nullString(input.Remark), Name: input.Name, Category: input.Category, Code: input.Code, Description: input.Description, Enabled: value, Vendor: input.Vendor, ID: id})
	if err != nil {
		return Application{}, translate(err)
	}
	return s.GetApplication(ctx, id)
}
func (s *Service) DeleteApplication(ctx context.Context, id int64) error {
	return translate(s.repository.DeleteApplication(ctx, id))
}
func (s *Service) ListVersions(ctx context.Context, applicationID int64, search string, page pagination.Page) ([]ApplicationVersion, int64, error) {
	rows, count, err := s.repository.ListVersions(ctx, applicationID, search, page)
	result := make([]ApplicationVersion, 0, len(rows))
	for _, row := range rows {
		result = append(result, versionList(row))
	}
	return result, count, translate(err)
}
func (s *Service) GetVersion(ctx context.Context, id int64) (ApplicationVersion, error) {
	row, err := s.repository.GetVersion(ctx, id)
	return versionDetail(row), translate(err)
}
func (s *Service) CreateVersion(ctx context.Context, input ApplicationVersionInput) (ApplicationVersion, error) {
	if input.Application < 1 || strings.TrimSpace(input.Version) == "" {
		return ApplicationVersion{}, ErrInvalid
	}
	now := time.Now().UTC()
	id, err := s.repository.CreateVersion(ctx, db.CreateApplicationVersionParams{CreateTime: now, UpdateTime: now, Remark: nullString(input.Remark), Version: strings.TrimSpace(input.Version), ReleaseDate: dateValue(input.ReleaseDate), EndOfSupport: dateValue(input.EndOfSupport), Enabled: enabled(input.Enabled), ApplicationID: input.Application})
	if err != nil {
		return ApplicationVersion{}, translate(err)
	}
	return s.GetVersion(ctx, id)
}
func (s *Service) UpdateVersion(ctx context.Context, id int64, input ApplicationVersionInput) (ApplicationVersion, error) {
	current, err := s.repository.GetVersion(ctx, id)
	if err != nil {
		return ApplicationVersion{}, translate(err)
	}
	if input.Application < 1 {
		input.Application = current.ApplicationID
	}
	if input.Version == "" {
		input.Version = current.Version
	}
	if input.Remark == nil {
		input.Remark = stringValue(current.Remark)
	}
	value := current.Enabled
	if input.Enabled != nil {
		value = *input.Enabled
	}
	err = s.repository.UpdateVersion(ctx, db.UpdateApplicationVersionParams{UpdateTime: time.Now().UTC(), Remark: nullString(input.Remark), Version: input.Version, ReleaseDate: dateOrCurrent(input.ReleaseDate, current.ReleaseDate), EndOfSupport: dateOrCurrent(input.EndOfSupport, current.EndOfSupport), Enabled: value, ApplicationID: input.Application, ID: id})
	if err != nil {
		return ApplicationVersion{}, translate(err)
	}
	return s.GetVersion(ctx, id)
}
func (s *Service) DeleteVersion(ctx context.Context, id int64) error {
	return translate(s.repository.DeleteVersion(ctx, id))
}
func (s *Service) ListProfiles(ctx context.Context, applicationID int64, search string, page pagination.Page) ([]ClusterProfile, int64, error) {
	rows, count, err := s.repository.ListProfiles(ctx, applicationID, search, page)
	result := make([]ClusterProfile, 0, len(rows))
	for _, row := range rows {
		result = append(result, profileList(row))
	}
	return result, count, translate(err)
}
func (s *Service) GetProfile(ctx context.Context, id int64) (ClusterProfile, error) {
	row, err := s.repository.GetProfile(ctx, id)
	return profileDetail(row), translate(err)
}
func (s *Service) CreateProfile(ctx context.Context, input ClusterProfileInput) (ClusterProfile, error) {
	if !validNamed(input.Name, input.Code) {
		return ClusterProfile{}, ErrInvalid
	}
	now := time.Now().UTC()
	id, err := s.repository.CreateProfile(ctx, db.CreateClusterProfileParams{CreateTime: now, UpdateTime: now, Remark: nullString(input.Remark), Name: input.Name, Code: input.Code, ProfileType: input.ProfileType, Enabled: enabled(input.Enabled), ApplicationID: nullInt(input.Application), ClusterType: input.ClusterType})
	if err != nil {
		return ClusterProfile{}, translate(err)
	}
	return s.GetProfile(ctx, id)
}
func (s *Service) UpdateProfile(ctx context.Context, id int64, input ClusterProfileInput) (ClusterProfile, error) {
	current, err := s.repository.GetProfile(ctx, id)
	if err != nil {
		return ClusterProfile{}, translate(err)
	}
	if current.ProfileType == "builtin" {
		return ClusterProfile{}, ErrDeleteProtected
	}
	if input.Name == "" {
		input.Name = current.Name
	}
	if input.Code == "" {
		input.Code = current.Code
	}
	if input.ProfileType == "" {
		input.ProfileType = current.ProfileType
	}
	if input.ClusterType == "" {
		input.ClusterType = current.ClusterType
	}
	if input.Application == nil {
		input.Application = intValue(current.ApplicationID)
	}
	if input.Remark == nil {
		input.Remark = stringValue(current.Remark)
	}
	value := current.Enabled
	if input.Enabled != nil {
		value = *input.Enabled
	}
	err = s.repository.UpdateProfile(ctx, db.UpdateClusterProfileParams{UpdateTime: time.Now().UTC(), Remark: nullString(input.Remark), Name: input.Name, Code: input.Code, ProfileType: input.ProfileType, Enabled: value, ApplicationID: nullInt(input.Application), ClusterType: input.ClusterType, ID: id})
	if err != nil {
		return ClusterProfile{}, translate(err)
	}
	return s.GetProfile(ctx, id)
}
func (s *Service) DeleteProfile(ctx context.Context, id int64) error {
	current, err := s.repository.GetProfile(ctx, id)
	if err != nil {
		return translate(err)
	}
	if current.ProfileType == "builtin" {
		return ErrDeleteProtected
	}
	return translate(s.repository.DeleteProfile(ctx, id))
}

func applicationList(r db.ListApplicationsRow) Application {
	return Application{ID: r.ID, CreateTime: timestamp(r.CreateTime), UpdateTime: timestamp(r.UpdateTime), Remark: stringValue(r.Remark), Name: r.Name, Code: r.Code, Category: r.Category, Vendor: r.Vendor, Description: r.Description, Enabled: r.Enabled, Versions: []ApplicationVersion{}, VersionCount: r.VersionCount, DeploymentTemplateCount: r.DeploymentTemplateCount, DeploymentCount: r.DeploymentCount}
}
func applicationDetail(r db.GetApplicationRow) Application {
	return Application{ID: r.ID, CreateTime: timestamp(r.CreateTime), UpdateTime: timestamp(r.UpdateTime), Remark: stringValue(r.Remark), Name: r.Name, Code: r.Code, Category: r.Category, Vendor: r.Vendor, Description: r.Description, Enabled: r.Enabled, Versions: []ApplicationVersion{}, VersionCount: r.VersionCount, DeploymentTemplateCount: r.DeploymentTemplateCount, DeploymentCount: r.DeploymentCount}
}
func versionList(r db.ListApplicationVersionsRow) ApplicationVersion {
	return ApplicationVersion{ID: r.ID, CreateTime: timestamp(r.CreateTime), UpdateTime: timestamp(r.UpdateTime), Remark: stringValue(r.Remark), Application: r.ApplicationID, ApplicationName: r.ApplicationName, Version: r.Version, ReleaseDate: datePtr(r.ReleaseDate), EndOfSupport: datePtr(r.EndOfSupport), Enabled: r.Enabled}
}
func versionDetail(r db.GetApplicationVersionRow) ApplicationVersion {
	return ApplicationVersion{ID: r.ID, CreateTime: timestamp(r.CreateTime), UpdateTime: timestamp(r.UpdateTime), Remark: stringValue(r.Remark), Application: r.ApplicationID, ApplicationName: r.ApplicationName, Version: r.Version, ReleaseDate: datePtr(r.ReleaseDate), EndOfSupport: datePtr(r.EndOfSupport), Enabled: r.Enabled}
}
func profileList(r db.ListClusterProfilesRow) ClusterProfile {
	return ClusterProfile{ID: r.ID, CreateTime: timestamp(r.CreateTime), UpdateTime: timestamp(r.UpdateTime), Remark: stringValue(r.Remark), Name: r.Name, Code: r.Code, ProfileType: r.ProfileType, ClusterType: r.ClusterType, Enabled: r.Enabled, Application: intValue(r.ApplicationID), ApplicationName: r.ApplicationName, ServiceCount: r.ServiceCount}
}
func profileDetail(r db.GetClusterProfileRow) ClusterProfile {
	return ClusterProfile{ID: r.ID, CreateTime: timestamp(r.CreateTime), UpdateTime: timestamp(r.UpdateTime), Remark: stringValue(r.Remark), Name: r.Name, Code: r.Code, ProfileType: r.ProfileType, ClusterType: r.ClusterType, Enabled: r.Enabled, Application: intValue(r.ApplicationID), ApplicationName: r.ApplicationName, ServiceCount: r.ServiceCount}
}
func dateValue(value *string) sql.NullTime {
	if value == nil || *value == "" {
		return sql.NullTime{}
	}
	parsed, err := time.Parse(time.DateOnly, *value)
	return sql.NullTime{Time: parsed, Valid: err == nil}
}
func dateOrCurrent(value *string, current sql.NullTime) sql.NullTime {
	if value == nil {
		return current
	}
	return dateValue(value)
}
func datePtr(value sql.NullTime) *string {
	if !value.Valid {
		return nil
	}
	text := value.Time.Format(time.DateOnly)
	return &text
}
