package sysconfig

import (
	"autoadmin/internal/identity"
	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/pagination"
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

const secretMask = "******"

type Config struct {
	ID           int64   `json:"id"`
	Key          string  `json:"key"`
	Value        any     `json:"value"`
	DefaultValue any     `json:"default_value"`
	ValueType    string  `json:"value_type"`
	Name         string  `json:"name"`
	Description  *string `json:"description"`
	IsReadonly   bool    `json:"is_readonly"`
	CreateTime   string  `json:"create_time"`
	UpdateTime   string  `json:"update_time"`
	Remark       *string `json:"remark"`
}
type Service struct{ repository *Repository }

func NewService(repository *Repository) *Service { return &Service{repository: repository} }
func (s *Service) List(ctx context.Context, search string, page pagination.Page) ([]Config, int64, error) {
	rows, count, err := s.repository.List(ctx, search, page)
	if err != nil {
		return nil, 0, err
	}
	result := make([]Config, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapConfig(row))
	}
	return result, count, nil
}
func (s *Service) GetByID(ctx context.Context, id int64) (Config, error) {
	row, err := s.repository.GetByID(ctx, id)
	return mapConfig(row), err
}
func (s *Service) GetByKey(ctx context.Context, key string) (Config, error) {
	row, err := s.repository.GetByKey(ctx, key)
	return mapConfig(row), err
}
func (s *Service) Update(ctx context.Context, id int64, value any, defaultValue any, allowDefault bool) (Config, error) {
	row, err := s.repository.GetByID(ctx, id)
	if err != nil {
		return Config{}, err
	}
	if row.IsReadonly {
		return Config{}, ErrReadonly
	}
	normalized, err := normalize(row.ValueType, value)
	if err != nil {
		return Config{}, err
	}
	newDefault := row.DefaultValue
	if defaultValue != nil && allowDefault {
		defaultText, err := normalize(row.ValueType, defaultValue)
		if err != nil {
			return Config{}, err
		}
		newDefault = sql.NullString{String: defaultText, Valid: true}
	}
	if row.ValueType == "secret" && normalized == secretMask {
		normalized = row.Value
	}
	err = s.repository.Update(ctx, db.UpdateConfigValueParams{Value: normalized, DefaultValue: newDefault, UpdateTime: time.Now().UTC(), ID: id})
	if err != nil {
		return Config{}, err
	}
	return s.GetByID(ctx, id)
}
func (s *Service) UpdateByKey(ctx context.Context, key string, value any) (Config, error) {
	row, err := s.repository.GetByKey(ctx, key)
	if err != nil {
		return Config{}, err
	}
	return s.Update(ctx, row.ID, value, nil, false)
}
func (s *Service) Reset(ctx context.Context, id int64) (Config, error) {
	row, err := s.repository.GetByID(ctx, id)
	if err != nil {
		return Config{}, err
	}
	if row.IsReadonly {
		return Config{}, ErrReadonly
	}
	if !row.DefaultValue.Valid {
		return Config{}, ErrNoDefault
	}
	value := row.DefaultValue.String
	if row.ValueType == "secret" {
		value, err = identity.HashPassword(value)
		if err != nil {
			return Config{}, err
		}
	}
	err = s.repository.Update(ctx, db.UpdateConfigValueParams{Value: value, DefaultValue: row.DefaultValue, UpdateTime: time.Now().UTC(), ID: id})
	if err != nil {
		return Config{}, err
	}
	return s.GetByID(ctx, id)
}
func normalize(kind string, value any) (string, error) {
	if kind == "secret" {
		text := strings.TrimSpace(toString(value))
		if text == secretMask {
			return secretMask, nil
		}
		return identity.HashPassword(text)
	}
	switch kind {
	case "int":
		number, err := strconv.Atoi(toString(value))
		if err != nil {
			return "", ErrValueNotInteger
		}
		return strconv.Itoa(number), nil
	case "bool":
		text := strings.ToLower(toString(value))
		if text == "true" || text == "1" || text == "yes" {
			return "true", nil
		}
		if text == "false" || text == "0" || text == "no" {
			return "false", nil
		}
		return "", ErrValueNotBoolean
	case "json":
		bytes, err := json.Marshal(value)
		if raw, ok := value.(string); ok {
			var parsed any
			if json.Unmarshal([]byte(raw), &parsed) != nil {
				return "", ErrValueNotJSON
			}
			bytes = []byte(raw)
		}
		return string(bytes), err
	default:
		return toString(value), nil
	}
}
func toString(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	bytes, _ := json.Marshal(value)
	return string(bytes)
}
func typed(kind, value string) any {
	switch kind {
	case "secret":
		if value != "" {
			return secretMask
		}
		return ""
	case "int":
		number, err := strconv.Atoi(value)
		if err == nil {
			return number
		}
	case "bool":
		return strings.ToLower(value) == "true" || value == "1" || strings.ToLower(value) == "yes"
	case "json":
		var parsed any
		if json.Unmarshal([]byte(value), &parsed) == nil {
			return parsed
		}
	}
	return value
}
func mapConfig(row db.SysConfig) Config {
	var defaultValue any
	if row.DefaultValue.Valid {
		defaultValue = typed(row.ValueType, row.DefaultValue.String)
	}
	return Config{ID: row.ID, Key: row.Key, Value: typed(row.ValueType, row.Value), DefaultValue: defaultValue, ValueType: row.ValueType, Name: row.Name, Description: stringPointer(row.Description), IsReadonly: row.IsReadonly, CreateTime: row.CreateTime.UTC().Format("2006-01-02T15:04:05.999999Z"), UpdateTime: row.UpdateTime.UTC().Format("2006-01-02T15:04:05.999999Z"), Remark: stringPointer(row.Remark)}
}
func stringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}
