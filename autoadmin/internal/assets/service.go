package assets

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"strings"
	"time"

	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/pagination"

	"github.com/go-sql-driver/mysql"
)

type Service struct {
	repository *Repository
	encryptor  *secretEncryptor
}

func NewService(repository *Repository, encryptionKey, djangoSecret string) (*Service, error) {
	encryptor, err := newSecretEncryptor(encryptionKey, djangoSecret)
	if err != nil {
		return nil, err
	}
	return &Service{repository: repository, encryptor: encryptor}, nil
}

func translate(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	var mysqlError *mysql.MySQLError
	if errors.As(err, &mysqlError) {
		switch mysqlError.Number {
		case 1062:
			return ErrDuplicate
		case 1451:
			return ErrDeleteProtected
		case 1452:
			return ErrInvalidRelation
		}
	}
	return err
}
func validNamed(name, code string) bool {
	return strings.TrimSpace(name) != "" && strings.TrimSpace(code) != ""
}

func project(row db.AssetsProject) Project {
	return Project{ID: row.ID, CreateTime: timestamp(row.CreateTime), UpdateTime: timestamp(row.UpdateTime), Remark: stringValue(row.Remark), Name: row.Name, Code: row.Code, Owner: row.Owner, Enabled: row.Enabled}
}
func (s *Service) ListProjects(ctx context.Context, search string, page pagination.Page) ([]Project, int64, error) {
	rows, count, err := s.repository.ListProjects(ctx, search, page)
	result := make([]Project, 0, len(rows))
	for _, row := range rows {
		result = append(result, project(row))
	}
	return result, count, translate(err)
}
func (s *Service) GetProject(ctx context.Context, id int64) (Project, error) {
	row, err := s.repository.GetProject(ctx, id)
	return project(row), translate(err)
}
func (s *Service) CreateProject(ctx context.Context, input ProjectInput) (Project, error) {
	if !validNamed(input.Name, input.Code) {
		return Project{}, ErrInvalid
	}
	now := time.Now().UTC()
	id, err := s.repository.CreateProject(ctx, db.CreateProjectParams{CreateTime: now, UpdateTime: now, Remark: nullString(input.Remark), Name: strings.TrimSpace(input.Name), Code: strings.TrimSpace(input.Code), Owner: strings.TrimSpace(input.Owner), Enabled: enabled(input.Enabled)})
	if err != nil {
		return Project{}, translate(err)
	}
	return s.GetProject(ctx, id)
}
func (s *Service) UpdateProject(ctx context.Context, id int64, input ProjectInput) (Project, error) {
	current, err := s.repository.GetProject(ctx, id)
	if err != nil {
		return Project{}, translate(err)
	}
	if strings.TrimSpace(input.Name) == "" {
		input.Name = current.Name
	}
	if strings.TrimSpace(input.Code) == "" {
		input.Code = current.Code
	}
	if input.Owner == "" {
		input.Owner = current.Owner
	}
	if input.Remark == nil {
		input.Remark = stringValue(current.Remark)
	}
	value := current.Enabled
	if input.Enabled != nil {
		value = *input.Enabled
	}
	err = s.repository.UpdateProject(ctx, db.UpdateProjectParams{UpdateTime: time.Now().UTC(), Remark: nullString(input.Remark), Name: strings.TrimSpace(input.Name), Code: strings.TrimSpace(input.Code), Owner: strings.TrimSpace(input.Owner), Enabled: value, ID: id})
	if err != nil {
		return Project{}, translate(err)
	}
	return s.GetProject(ctx, id)
}
func (s *Service) DeleteProject(ctx context.Context, id int64) error {
	if _, err := s.repository.GetProject(ctx, id); err != nil {
		return translate(err)
	}
	return translate(s.repository.DeleteProject(ctx, id))
}

func businessSystem(row db.ListBusinessSystemsRow) BusinessSystem {
	return BusinessSystem{ID: row.ID, CreateTime: timestamp(row.CreateTime), UpdateTime: timestamp(row.UpdateTime), Remark: stringValue(row.Remark), Name: row.Name, Code: row.Code, Owner: row.Owner, Enabled: row.Enabled, Project: intValue(row.ProjectID), ProjectName: row.ProjectName, ProjectCode: row.ProjectCode}
}
func businessSystemDetail(row db.GetBusinessSystemRow) BusinessSystem {
	return BusinessSystem{ID: row.ID, CreateTime: timestamp(row.CreateTime), UpdateTime: timestamp(row.UpdateTime), Remark: stringValue(row.Remark), Name: row.Name, Code: row.Code, Owner: row.Owner, Enabled: row.Enabled, Project: intValue(row.ProjectID), ProjectName: row.ProjectName, ProjectCode: row.ProjectCode}
}
func (s *Service) ListBusinessSystems(ctx context.Context, search string, page pagination.Page) ([]BusinessSystem, int64, error) {
	rows, count, err := s.repository.ListBusinessSystems(ctx, search, page)
	result := make([]BusinessSystem, 0, len(rows))
	for _, row := range rows {
		result = append(result, businessSystem(row))
	}
	return result, count, translate(err)
}
func (s *Service) GetBusinessSystem(ctx context.Context, id int64) (BusinessSystem, error) {
	row, err := s.repository.GetBusinessSystem(ctx, id)
	return businessSystemDetail(row), translate(err)
}
func (s *Service) validateProject(ctx context.Context, id *int64) error {
	if id == nil {
		return ErrInvalidRelation
	}
	_, err := s.repository.GetProject(ctx, *id)
	if err != nil {
		return ErrInvalidRelation
	}
	return nil
}
func (s *Service) CreateBusinessSystem(ctx context.Context, input BusinessSystemInput) (BusinessSystem, error) {
	if !validNamed(input.Name, input.Code) || s.validateProject(ctx, input.Project) != nil {
		return BusinessSystem{}, ErrInvalidRelation
	}
	now := time.Now().UTC()
	id, err := s.repository.CreateBusinessSystem(ctx, db.CreateBusinessSystemParams{CreateTime: now, UpdateTime: now, Remark: nullString(input.Remark), Name: strings.TrimSpace(input.Name), Code: strings.TrimSpace(input.Code), Owner: strings.TrimSpace(input.Owner), Enabled: enabled(input.Enabled), ProjectID: nullInt(input.Project)})
	if err != nil {
		return BusinessSystem{}, translate(err)
	}
	return s.GetBusinessSystem(ctx, id)
}
func (s *Service) UpdateBusinessSystem(ctx context.Context, id int64, input BusinessSystemInput) (BusinessSystem, error) {
	current, err := s.repository.GetBusinessSystem(ctx, id)
	if err != nil {
		return BusinessSystem{}, translate(err)
	}
	if strings.TrimSpace(input.Name) == "" {
		input.Name = current.Name
	}
	if strings.TrimSpace(input.Code) == "" {
		input.Code = current.Code
	}
	if input.Owner == "" {
		input.Owner = current.Owner
	}
	if input.Project == nil {
		input.Project = intValue(current.ProjectID)
	}
	if input.Remark == nil {
		input.Remark = stringValue(current.Remark)
	}
	if !validNamed(input.Name, input.Code) || s.validateProject(ctx, input.Project) != nil {
		return BusinessSystem{}, ErrInvalidRelation
	}
	value := current.Enabled
	if input.Enabled != nil {
		value = *input.Enabled
	}
	err = s.repository.UpdateBusinessSystem(ctx, db.UpdateBusinessSystemParams{UpdateTime: time.Now().UTC(), Remark: nullString(input.Remark), Name: strings.TrimSpace(input.Name), Code: strings.TrimSpace(input.Code), Owner: strings.TrimSpace(input.Owner), Enabled: value, ProjectID: nullInt(input.Project), ID: id})
	if err != nil {
		return BusinessSystem{}, translate(err)
	}
	return s.GetBusinessSystem(ctx, id)
}
func (s *Service) DeleteBusinessSystem(ctx context.Context, id int64) error {
	if _, err := s.repository.GetBusinessSystem(ctx, id); err != nil {
		return translate(err)
	}
	return translate(s.repository.DeleteBusinessSystem(ctx, id))
}

func environment(row db.AssetsBusinessEnvironment) BusinessEnvironment {
	return BusinessEnvironment{ID: row.ID, CreateTime: timestamp(row.CreateTime), UpdateTime: timestamp(row.UpdateTime), Remark: stringValue(row.Remark), Name: row.Name, Code: row.Code, Order: row.Order, Owner: row.Owner, Enabled: row.Enabled}
}
func (s *Service) ListEnvironments(ctx context.Context, search string, page pagination.Page) ([]BusinessEnvironment, int64, error) {
	rows, count, err := s.repository.ListEnvironments(ctx, search, page)
	result := make([]BusinessEnvironment, 0, len(rows))
	for _, row := range rows {
		result = append(result, environment(row))
	}
	return result, count, translate(err)
}
func (s *Service) GetEnvironment(ctx context.Context, id int64) (BusinessEnvironment, error) {
	row, err := s.repository.GetEnvironment(ctx, id)
	return environment(row), translate(err)
}
func (s *Service) CreateEnvironment(ctx context.Context, input EnvironmentInput) (BusinessEnvironment, error) {
	if !validNamed(input.Name, input.Code) {
		return BusinessEnvironment{}, ErrInvalid
	}
	now := time.Now().UTC()
	id, err := s.repository.CreateEnvironment(ctx, db.CreateBusinessEnvironmentParams{CreateTime: now, UpdateTime: now, Remark: nullString(input.Remark), Name: strings.TrimSpace(input.Name), Code: strings.TrimSpace(input.Code), Order: input.Order, Owner: strings.TrimSpace(input.Owner), Enabled: enabled(input.Enabled)})
	if err != nil {
		return BusinessEnvironment{}, translate(err)
	}
	return s.GetEnvironment(ctx, id)
}
func (s *Service) UpdateEnvironment(ctx context.Context, id int64, input EnvironmentInput) (BusinessEnvironment, error) {
	current, err := s.repository.GetEnvironment(ctx, id)
	if err != nil {
		return BusinessEnvironment{}, translate(err)
	}
	if strings.TrimSpace(input.Name) == "" {
		input.Name = current.Name
	}
	if strings.TrimSpace(input.Code) == "" {
		input.Code = current.Code
	}
	if input.Owner == "" {
		input.Owner = current.Owner
	}
	if input.Remark == nil {
		input.Remark = stringValue(current.Remark)
	}
	if input.Order == 0 {
		input.Order = current.Order
	}
	if !validNamed(input.Name, input.Code) {
		return BusinessEnvironment{}, ErrInvalid
	}
	value := current.Enabled
	if input.Enabled != nil {
		value = *input.Enabled
	}
	err = s.repository.UpdateEnvironment(ctx, db.UpdateBusinessEnvironmentParams{UpdateTime: time.Now().UTC(), Remark: nullString(input.Remark), Name: strings.TrimSpace(input.Name), Code: strings.TrimSpace(input.Code), Order: input.Order, Owner: strings.TrimSpace(input.Owner), Enabled: value, ID: id})
	if err != nil {
		return BusinessEnvironment{}, translate(err)
	}
	return s.GetEnvironment(ctx, id)
}
func (s *Service) DeleteEnvironment(ctx context.Context, id int64) error {
	if _, err := s.repository.GetEnvironment(ctx, id); err != nil {
		return translate(err)
	}
	return translate(s.repository.DeleteEnvironment(ctx, id))
}

func credential(row db.AssetsCredential) Credential {
	password, privateKey := stringValue(row.Password), stringValue(row.PrivateKey)
	if password != nil && *password != "" {
		masked := secretMask
		password = &masked
	}
	if privateKey != nil && *privateKey != "" {
		masked := secretMask
		privateKey = &masked
	}
	return Credential{ID: row.ID, CreateTime: timestamp(row.CreateTime), UpdateTime: timestamp(row.UpdateTime), Remark: stringValue(row.Remark), Name: stringValue(row.Name), Password: password, PrivateKey: privateKey, AuthType: row.AuthType, Username: row.Username, Port: row.Port}
}
func (s *Service) ListCredentials(ctx context.Context, search string, page pagination.Page) ([]Credential, int64, error) {
	rows, count, err := s.repository.ListCredentials(ctx, search, page)
	result := make([]Credential, 0, len(rows))
	for _, row := range rows {
		result = append(result, credential(row))
	}
	return result, count, translate(err)
}
func (s *Service) GetCredential(ctx context.Context, id int64) (Credential, error) {
	row, err := s.repository.GetCredential(ctx, id)
	return credential(row), translate(err)
}
func (s *Service) credentialParams(input CredentialInput, current *db.AssetsCredential) (sql.NullString, sql.NullString, error) {
	password := nullString(input.Password)
	privateKey := nullString(input.PrivateKey)
	if current != nil {
		if input.Password == nil || *input.Password == secretMask {
			password = current.Password
		}
		if input.PrivateKey == nil || *input.PrivateKey == secretMask {
			privateKey = current.PrivateKey
		}
	}
	if password.Valid && password.String != "" && (!strings.HasPrefix(password.String, encryptedPrefix)) {
		encrypted, err := s.encryptor.Encrypt(password.String)
		if err != nil {
			return sql.NullString{}, sql.NullString{}, err
		}
		password.String = encrypted
	}
	return password, privateKey, nil
}
func validateCredential(input CredentialInput) bool {
	return strings.TrimSpace(input.Username) != "" && (input.AuthType == 1 || input.AuthType == 2) && input.Port > 0 && input.Port <= 65535 && (input.PrivateKey == nil || len(*input.PrivateKey) <= 8192)
}
func (s *Service) CreateCredential(ctx context.Context, input CredentialInput) (Credential, error) {
	if !validateCredential(input) {
		return Credential{}, ErrInvalid
	}
	password, privateKey, err := s.credentialParams(input, nil)
	if err != nil {
		return Credential{}, err
	}
	now := time.Now().UTC()
	id, err := s.repository.CreateCredential(ctx, db.CreateCredentialParams{CreateTime: now, UpdateTime: now, Remark: nullString(input.Remark), Name: nullString(input.Name), Password: password, PrivateKey: privateKey, AuthType: input.AuthType, Username: strings.TrimSpace(input.Username), Port: input.Port})
	if err != nil {
		return Credential{}, translate(err)
	}
	return s.GetCredential(ctx, id)
}
func (s *Service) UpdateCredential(ctx context.Context, id int64, input CredentialInput) (Credential, error) {
	current, err := s.repository.GetCredential(ctx, id)
	if err != nil {
		return Credential{}, translate(err)
	}
	if input.Name == nil {
		input.Name = stringValue(current.Name)
	}
	if input.Username == "" {
		input.Username = current.Username
	}
	if input.Port == 0 {
		input.Port = current.Port
	}
	if input.AuthType == 0 {
		input.AuthType = current.AuthType
	}
	if input.Remark == nil {
		input.Remark = stringValue(current.Remark)
	}
	if !validateCredential(input) {
		return Credential{}, ErrInvalid
	}
	password, privateKey, err := s.credentialParams(input, &current)
	if err != nil {
		return Credential{}, err
	}
	err = s.repository.UpdateCredential(ctx, db.UpdateCredentialParams{UpdateTime: time.Now().UTC(), Remark: nullString(input.Remark), Name: nullString(input.Name), Password: password, PrivateKey: privateKey, AuthType: input.AuthType, Username: strings.TrimSpace(input.Username), Port: input.Port, ID: id})
	if err != nil {
		return Credential{}, translate(err)
	}
	return s.GetCredential(ctx, id)
}
func (s *Service) DeleteCredential(ctx context.Context, id int64) error {
	if _, err := s.repository.GetCredential(ctx, id); err != nil {
		return translate(err)
	}
	return translate(s.repository.DeleteCredential(ctx, id))
}
func (s *Service) DeleteCredentials(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return ErrInvalid
	}
	return translate(s.repository.DeleteCredentials(ctx, ids))
}

func hostGroup(row db.ListHostGroupsRow) HostGroup {
	parent := intValue(row.ParentID)
	return HostGroup{ID: row.ID, CreateTime: timestamp(row.CreateTime), UpdateTime: timestamp(row.UpdateTime), Remark: stringValue(row.Remark), Name: row.Name, Parent: parent, ParentID: parent, ParentName: row.ParentName, HostCount: row.HostCount}
}
func hostGroupDetail(row db.GetHostGroupRow) HostGroup {
	parent := intValue(row.ParentID)
	return HostGroup{ID: row.ID, CreateTime: timestamp(row.CreateTime), UpdateTime: timestamp(row.UpdateTime), Remark: stringValue(row.Remark), Name: row.Name, Parent: parent, ParentID: parent, ParentName: row.ParentName, HostCount: row.HostCount}
}
func (s *Service) ListHostGroups(ctx context.Context, search string, page pagination.Page) ([]HostGroup, int64, error) {
	rows, count, err := s.repository.ListHostGroups(ctx, search, page)
	result := make([]HostGroup, 0, len(rows))
	for _, row := range rows {
		result = append(result, hostGroup(row))
	}
	return result, count, translate(err)
}
func (s *Service) GetHostGroup(ctx context.Context, id int64) (HostGroup, error) {
	row, err := s.repository.GetHostGroup(ctx, id)
	return hostGroupDetail(row), translate(err)
}
func (s *Service) HostGroupTree(ctx context.Context) ([]HostGroup, error) {
	rows, err := s.repository.ListAllHostGroups(ctx)
	if err != nil {
		return nil, translate(err)
	}
	return buildHostGroupTree(rows), nil
}

func buildHostGroupTree(rows []db.ListAllHostGroupsRow) []HostGroup {
	children := make(map[int64][]HostGroup)
	for _, row := range rows {
		parent := intValue(row.ParentID)
		item := HostGroup{ID: row.ID, CreateTime: timestamp(row.CreateTime), UpdateTime: timestamp(row.UpdateTime), Remark: stringValue(row.Remark), Name: row.Name, Parent: parent, ParentID: parent, ParentName: row.ParentName, HostCount: row.HostCount}
		parentID := int64(0)
		if parent != nil {
			parentID = *parent
		}
		children[parentID] = append(children[parentID], item)
	}
	var build func(int64, map[int64]bool) ([]HostGroup, int64)
	build = func(parentID int64, ancestors map[int64]bool) ([]HostGroup, int64) {
		result := make([]HostGroup, 0, len(children[parentID]))
		total := int64(0)
		for _, item := range children[parentID] {
			if ancestors[item.ID] {
				continue
			}
			next := make(map[int64]bool, len(ancestors)+1)
			for id := range ancestors {
				next[id] = true
			}
			next[item.ID] = true
			item.Children, item.HostCount = build(item.ID, next)
			item.HostCount += directHostCount(rows, item.ID)
			total += item.HostCount
			result = append(result, item)
		}
		return result, total
	}
	tree, _ := build(0, map[int64]bool{})
	return tree
}

func directHostCount(rows []db.ListAllHostGroupsRow, id int64) int64 {
	for _, row := range rows {
		if row.ID == id {
			return row.HostCount
		}
	}
	return 0
}
func (s *Service) validateGroupParent(ctx context.Context, id int64, parent *int64) error {
	if parent == nil {
		return nil
	}
	if *parent == id {
		return ErrGroupCycle
	}
	current := *parent
	for depth := 0; depth < 64; depth++ {
		row, err := s.repository.GetHostGroup(ctx, current)
		if err != nil {
			return ErrInvalidRelation
		}
		if !row.ParentID.Valid {
			return nil
		}
		current = row.ParentID.Int64
		if current == id {
			return ErrGroupCycle
		}
	}
	return ErrGroupCycle
}
func (s *Service) CreateHostGroup(ctx context.Context, input HostGroupInput) (HostGroup, error) {
	if strings.TrimSpace(input.Name) == "" {
		return HostGroup{}, ErrInvalid
	}
	if err := s.validateGroupParent(ctx, 0, input.ParentID); err != nil {
		return HostGroup{}, err
	}
	now := time.Now().UTC()
	id, err := s.repository.CreateHostGroup(ctx, db.CreateHostGroupParams{CreateTime: now, UpdateTime: now, Remark: nullString(input.Remark), Name: strings.TrimSpace(input.Name), ParentID: nullInt(input.ParentID)})
	if err != nil {
		return HostGroup{}, translate(err)
	}
	return s.GetHostGroup(ctx, id)
}
func (s *Service) UpdateHostGroup(ctx context.Context, id int64, input HostGroupInput) (HostGroup, error) {
	current, err := s.repository.GetHostGroup(ctx, id)
	if err != nil {
		return HostGroup{}, translate(err)
	}
	if strings.TrimSpace(input.Name) == "" {
		input.Name = current.Name
	}
	if input.Remark == nil {
		input.Remark = stringValue(current.Remark)
	}
	if input.ParentID == nil {
		input.ParentID = intValue(current.ParentID)
	}
	if err := s.validateGroupParent(ctx, id, input.ParentID); err != nil {
		return HostGroup{}, err
	}
	err = s.repository.UpdateHostGroup(ctx, db.UpdateHostGroupParams{UpdateTime: time.Now().UTC(), Remark: nullString(input.Remark), Name: strings.TrimSpace(input.Name), ParentID: nullInt(input.ParentID), ID: id})
	if err != nil {
		return HostGroup{}, translate(err)
	}
	return s.GetHostGroup(ctx, id)
}
func (s *Service) DeleteHostGroup(ctx context.Context, id int64) error {
	if _, err := s.repository.GetHostGroup(ctx, id); err != nil {
		return translate(err)
	}
	return translate(s.repository.DeleteHostGroup(ctx, id))
}

func host(row db.ListHostsRow) Host {
	return Host{ID: row.ID, CreateTime: timestamp(row.CreateTime), UpdateTime: timestamp(row.UpdateTime), Remark: stringValue(row.Remark), Status: row.Status, InstanceID: stringValue(row.InstanceID), IP: stringValue(row.Ip), IsDeletedInCloud: row.IsDeletedInCloud, CloudAccount: intValue(row.CloudAccountID), Group: intValue(row.GroupID), GroupName: row.GroupName, InstanceName: stringValue(row.InstanceName), CollectStatus: row.CollectStatus, CollectMessage: row.CollectMessage, CollectTime: timeValue(row.CollectTime), AgentOnline: row.AgentOnline, AgentOnlineTime: timeValue(row.AgentOnlineTime), WebSSHDefaultUsername: row.WebsshDefaultUsername, WebSSHLoginUsers: row.WebsshLoginUsers, AgentID: stringValue(row.AgentID), Environment: intValue(row.EnvironmentID), EnvironmentName: row.EnvironmentName}
}
func hostDetail(row db.GetHostRow) Host {
	return Host{ID: row.ID, CreateTime: timestamp(row.CreateTime), UpdateTime: timestamp(row.UpdateTime), Remark: stringValue(row.Remark), Status: row.Status, InstanceID: stringValue(row.InstanceID), IP: stringValue(row.Ip), IsDeletedInCloud: row.IsDeletedInCloud, CloudAccount: intValue(row.CloudAccountID), Group: intValue(row.GroupID), GroupName: row.GroupName, InstanceName: stringValue(row.InstanceName), CollectStatus: row.CollectStatus, CollectMessage: row.CollectMessage, CollectTime: timeValue(row.CollectTime), AgentOnline: row.AgentOnline, AgentOnlineTime: timeValue(row.AgentOnlineTime), WebSSHDefaultUsername: row.WebsshDefaultUsername, WebSSHLoginUsers: row.WebsshLoginUsers, AgentID: stringValue(row.AgentID), Environment: intValue(row.EnvironmentID), EnvironmentName: row.EnvironmentName}
}
func (s *Service) ListHosts(ctx context.Context, search string, groupID, environmentID int64, page pagination.Page) ([]Host, int64, error) {
	rows, count, err := s.repository.ListHosts(ctx, search, groupID, environmentID, page)
	result := make([]Host, 0, len(rows))
	for _, row := range rows {
		result = append(result, host(row))
	}
	return result, count, translate(err)
}
func (s *Service) GetHost(ctx context.Context, id int64) (Host, error) {
	row, err := s.repository.GetHost(ctx, id)
	return hostDetail(row), translate(err)
}
func (s *Service) validateHostInput(ctx context.Context, input HostInput) error {
	if input.IP != nil && *input.IP != "" && net.ParseIP(*input.IP) == nil {
		return ErrInvalid
	}
	if input.GroupID != nil {
		if _, err := s.repository.GetHostGroup(ctx, *input.GroupID); err != nil {
			return ErrInvalidRelation
		}
	}
	if input.Environment != nil {
		if _, err := s.repository.GetEnvironment(ctx, *input.Environment); err != nil {
			return ErrInvalidRelation
		}
	}
	return nil
}
func (s *Service) validateHostPatch(ctx context.Context, patch HostPatchInput) error {
	if patch.IP.Present && patch.IP.Value != nil && *patch.IP.Value != "" && net.ParseIP(*patch.IP.Value) == nil {
		return ErrInvalid
	}
	if patch.GroupID.Present && patch.GroupID.Value != nil {
		if _, err := s.repository.GetHostGroup(ctx, *patch.GroupID.Value); err != nil {
			return ErrInvalidRelation
		}
	}
	if patch.Environment.Present && patch.Environment.Value != nil {
		if _, err := s.repository.GetEnvironment(ctx, *patch.Environment.Value); err != nil {
			return ErrInvalidRelation
		}
	}
	return nil
}
func normalizeWebSSH(input *HostInput) {
	users := strings.Fields(input.WebSSHLoginUsers)
	if len(users) == 0 {
		users = []string{"root"}
	}
	input.WebSSHLoginUsers = strings.Join(users, " ")
	if !contains(users, input.WebSSHDefaultUsername) {
		input.WebSSHDefaultUsername = users[0]
	}
}
func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
func mergePatchValue[T any](field PatchField[T], current *T) *T {
	if field.Present {
		return field.Value
	}
	return current
}
func mergeHostPatch(current db.GetHostRow, patch HostPatchInput) (HostInput, error) {
	status := current.Status
	if patch.Status.Present {
		if patch.Status.Value == nil {
			return HostInput{}, ErrInvalid
		}
		status = *patch.Status.Value
	}
	isDeletedInCloud := current.IsDeletedInCloud
	if patch.IsDeletedInCloud.Present {
		if patch.IsDeletedInCloud.Value == nil {
			return HostInput{}, ErrInvalid
		}
		isDeletedInCloud = *patch.IsDeletedInCloud.Value
	}
	webSSHDefaultUsername := current.WebsshDefaultUsername
	if patch.WebSSHDefaultUsername.Present {
		if patch.WebSSHDefaultUsername.Value == nil {
			return HostInput{}, ErrInvalid
		}
		webSSHDefaultUsername = *patch.WebSSHDefaultUsername.Value
	}
	webSSHLoginUsers := current.WebsshLoginUsers
	if patch.WebSSHLoginUsers.Present {
		if patch.WebSSHLoginUsers.Value == nil {
			return HostInput{}, ErrInvalid
		}
		webSSHLoginUsers = *patch.WebSSHLoginUsers.Value
	}
	return HostInput{
		InstanceName:          mergePatchValue(patch.InstanceName, stringValue(current.InstanceName)),
		AgentID:               mergePatchValue(patch.AgentID, stringValue(current.AgentID)),
		IP:                    mergePatchValue(patch.IP, stringValue(current.Ip)),
		InstanceID:            mergePatchValue(patch.InstanceID, stringValue(current.InstanceID)),
		Environment:           mergePatchValue(patch.Environment, intValue(current.EnvironmentID)),
		CloudAccount:          mergePatchValue(patch.CloudAccount, intValue(current.CloudAccountID)),
		GroupID:               mergePatchValue(patch.GroupID, intValue(current.GroupID)),
		Status:                status,
		IsDeletedInCloud:      isDeletedInCloud,
		WebSSHDefaultUsername: webSSHDefaultUsername,
		WebSSHLoginUsers:      webSSHLoginUsers,
		Remark:                mergePatchValue(patch.Remark, stringValue(current.Remark)),
	}, nil
}
func (s *Service) CreateHost(ctx context.Context, input HostInput) (Host, error) {
	if err := s.validateHostInput(ctx, input); err != nil {
		return Host{}, err
	}
	normalizeWebSSH(&input)
	if input.Status == "" {
		input.Status = "running"
	}
	now := time.Now().UTC()
	id, err := s.repository.CreateHost(ctx, db.CreateHostParams{CreateTime: now, UpdateTime: now, Remark: nullString(input.Remark), Status: input.Status, InstanceID: nullString(input.InstanceID), Ip: nullString(input.IP), IsDeletedInCloud: input.IsDeletedInCloud, CloudAccountID: nullInt(input.CloudAccount), GroupID: nullInt(input.GroupID), InstanceName: nullString(input.InstanceName), CollectStatus: "unknown", CollectMessage: "", WebsshDefaultUsername: input.WebSSHDefaultUsername, WebsshLoginUsers: input.WebSSHLoginUsers, AgentID: nullString(input.AgentID), EnvironmentID: nullInt(input.Environment)})
	if err != nil {
		return Host{}, translate(err)
	}
	return s.GetHost(ctx, id)
}
func (s *Service) UpdateHost(ctx context.Context, id int64, input HostInput) (Host, error) {
	current, err := s.repository.GetHost(ctx, id)
	if err != nil {
		return Host{}, translate(err)
	}
	if err = s.validateHostInput(ctx, input); err != nil {
		return Host{}, err
	}
	return s.updateHost(ctx, id, current, input)
}
func (s *Service) PatchHost(ctx context.Context, id int64, patch HostPatchInput) (Host, error) {
	current, err := s.repository.GetHost(ctx, id)
	if err != nil {
		return Host{}, translate(err)
	}
	if err = s.validateHostPatch(ctx, patch); err != nil {
		return Host{}, err
	}
	input, err := mergeHostPatch(current, patch)
	if err != nil {
		return Host{}, err
	}
	return s.updateHost(ctx, id, current, input)
}
func (s *Service) updateHost(ctx context.Context, id int64, current db.GetHostRow, input HostInput) (Host, error) {
	normalizeWebSSH(&input)
	if input.Status == "" {
		input.Status = current.Status
	}
	err := s.repository.UpdateHost(ctx, db.UpdateHostParams{UpdateTime: time.Now().UTC(), Remark: nullString(input.Remark), Status: input.Status, InstanceID: nullString(input.InstanceID), Ip: nullString(input.IP), IsDeletedInCloud: input.IsDeletedInCloud, CloudAccountID: nullInt(input.CloudAccount), GroupID: nullInt(input.GroupID), InstanceName: nullString(input.InstanceName), CollectStatus: current.CollectStatus, CollectMessage: current.CollectMessage, CollectTime: current.CollectTime, AgentOnline: current.AgentOnline, AgentOnlineTime: current.AgentOnlineTime, WebsshDefaultUsername: input.WebSSHDefaultUsername, WebsshLoginUsers: input.WebSSHLoginUsers, AgentID: nullString(input.AgentID), EnvironmentID: nullInt(input.Environment), ID: id})
	if err != nil {
		return Host{}, translate(err)
	}
	return s.GetHost(ctx, id)
}
func (s *Service) DeleteHost(ctx context.Context, id int64) error {
	return translate(s.repository.DeleteHost(ctx, id))
}
func (s *Service) DeleteHosts(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return ErrInvalid
	}
	return translate(s.repository.DeleteHosts(ctx, ids))
}
