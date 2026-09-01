package identity

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/pagination"
)

type UserInput struct {
	Username    string
	Phonenumber *string
	Status      int16
	Remark      *string
	Timezone    string
}

func (service *Service) UserDetail(ctx context.Context, userID int32) (CurrentUser, error) {
	return service.Current(ctx, userID)
}

func (service *Service) CreateUser(ctx context.Context, input UserInput) (CurrentUser, error) {
	password, err := HashPassword("123456")
	if err != nil {
		return CurrentUser{}, fmt.Errorf("hash default password: %w", err)
	}
	now := time.Now().UTC()
	user, err := service.repository.Create(ctx, db.CreateUserParams{
		Username: input.Username, Password: password,
		Phonenumber: nullableString(input.Phonenumber), Status: input.Status,
		CreateTime: sql.NullTime{Time: now, Valid: true}, UpdateTime: sql.NullTime{Time: now, Valid: true},
		Remark: nullableString(input.Remark), Timezone: input.Timezone,
	})
	if err != nil {
		return CurrentUser{}, err
	}
	return mapUser(user), nil
}

func (service *Service) UpdateUser(ctx context.Context, userID int32, input UserInput) (CurrentUser, error) {
	user, err := service.repository.GetByID(ctx, userID)
	if err != nil {
		return CurrentUser{}, err
	}
	if input.Phonenumber != nil {
		user.Phonenumber = nullableString(input.Phonenumber)
	}
	if input.Remark != nil {
		user.Remark = nullableString(input.Remark)
	}
	if input.Timezone != "" {
		user.Timezone = input.Timezone
	}
	user.Status = input.Status
	now := sql.NullTime{Time: time.Now().UTC(), Valid: true}
	if err := service.repository.Update(ctx, db.UpdateUserProfileParams{
		Avatar: user.Avatar, Phonenumber: user.Phonenumber, Status: user.Status,
		Remark: user.Remark, Timezone: user.Timezone, UpdateTime: now, ID: userID,
	}); err != nil {
		return CurrentUser{}, err
	}
	return service.Current(ctx, userID)
}

func (service *Service) ResetPassword(ctx context.Context, userID int32) error {
	if _, err := service.repository.GetByID(ctx, userID); err != nil {
		return err
	}
	password, err := HashPassword("123456")
	if err != nil {
		return err
	}
	return service.repository.UpdatePassword(ctx, db.UpdateUserPasswordParams{
		Password: password, UpdateTime: sql.NullTime{Time: time.Now().UTC(), Valid: true}, ID: userID,
	})
}

func (service *Service) ChangeStatus(ctx context.Context, userID int32, status int16) error {
	user, err := service.repository.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	_, err = service.UpdateUser(ctx, userID, UserInput{Status: status, Timezone: user.Timezone})
	return err
}

func (service *Service) DeleteUsers(ctx context.Context, userIDs []int32) error {
	return service.repository.DeleteMany(ctx, userIDs)
}

func (service *Service) UserRoles(ctx context.Context, userID int32) ([]Role, error) {
	rows, err := service.repository.ListRoles(ctx, userID)
	if err != nil {
		return nil, err
	}
	roles := make([]Role, 0, len(rows))
	for _, row := range rows {
		roles = append(roles, mapRole(row))
	}
	return roles, nil
}

func (service *Service) UsernameExists(ctx context.Context, username string) (bool, error) {
	_, err := service.repository.GetByUsername(ctx, username)
	if err == nil {
		return true, nil
	}
	if err == sql.ErrNoRows {
		return false, nil
	}
	return false, err
}

func nullableString(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}

var _ = pagination.Page{}
