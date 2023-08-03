package impl

import (
	"context"
	"fmt"
	"github.com/Suj8K/oxygen-go/services/db"
	"github.com/Suj8K/oxygen-go/services/user"
	"github.com/Suj8K/oxygen-go/util"
	"log"
	"strings"
	"time"
)

type Service struct {
	store                store
	caseInsensitiveLogin bool
}

func ProvideService(
	db db.DB,
) (user.Service, error) {
	store := ProvideStore(db)
	s := &Service{
		store: &store,
	}

	return s, nil
}

func (s *Service) GetUsageStats(ctx context.Context) map[string]interface{} {
	stats := map[string]interface{}{}
	caseInsensitiveLoginVal := 0
	if s.caseInsensitiveLogin {
		caseInsensitiveLoginVal = 1
	}

	stats["stats.case_insensitive_login.count"] = caseInsensitiveLoginVal
	return stats
}

func (s *Service) Create(ctx context.Context, cmd *user.CreateUserCommand) (*user.User, error) {
	err := s.store.LoginConflict(ctx, cmd.Login, cmd.Email, s.caseInsensitiveLogin)
	if err != nil {
		return nil, user.ErrUserAlreadyExists
	}

	// create user
	usr := &user.User{
		Email:            cmd.Email,
		Name:             cmd.Name,
		Login:            cmd.Login,
		Company:          cmd.Company,
		IsAdmin:          cmd.IsAdmin,
		IsDisabled:       cmd.IsDisabled,
		EmailVerified:    cmd.EmailVerified,
		Created:          time.Now(),
		Updated:          time.Now(),
		LastSeenAt:       time.Now().AddDate(-10, 0, 0),
		IsServiceAccount: cmd.IsServiceAccount,
	}

	salt, err := util.GetRandomString(10)
	if err != nil {
		return nil, err
	}
	usr.Salt = salt
	rands, err := util.GetRandomString(10)
	if err != nil {
		return nil, err
	}
	usr.Rands = rands

	if len(cmd.Password) > 0 {
		encodedPassword, err := util.EncodePassword(cmd.Password, usr.Salt)
		if err != nil {
			return nil, err
		}
		usr.Password = encodedPassword
	}

	_, err = s.store.Insert(ctx, usr)
	if err != nil {
		return nil, err
	}

	return usr, nil
}

func (s *Service) Delete(ctx context.Context, cmd *user.DeleteUserCommand) error {
	_, err := s.store.GetNotServiceAccount(ctx, cmd.UserID)
	if err != nil {
		return err
	}
	// delete from all the stores
	return s.store.Delete(ctx, cmd.UserID)
}

func (s *Service) GetByID(ctx context.Context, query *user.GetUserByIDQuery) (*user.User, error) {
	user, err := s.store.GetByID(ctx, query.ID)
	if err != nil {
		log.Println("Error in implementation")
		return nil, err
	}
	if s.caseInsensitiveLogin {
		if err := s.store.CaseInsensitiveLoginConflict(ctx, user.Login, user.Email); err != nil {
			return nil, err
		}
	}
	return user, nil
}

func (s *Service) GetByLogin(ctx context.Context, query *user.GetUserByLoginQuery) (*user.User, error) {
	return s.store.GetByLogin(ctx, query)
}

func (s *Service) GetByEmail(ctx context.Context, query *user.GetUserByEmailQuery) (*user.User, error) {
	return s.store.GetByEmail(ctx, query)
}

func (s *Service) Update(ctx context.Context, cmd *user.UpdateUserCommand) error {
	if s.caseInsensitiveLogin {
		cmd.Login = strings.ToLower(cmd.Login)
		cmd.Email = strings.ToLower(cmd.Email)
	}
	return s.store.Update(ctx, cmd)
}

func (s *Service) ChangePassword(ctx context.Context, cmd *user.ChangeUserPasswordCommand) error {
	return s.store.ChangePassword(ctx, cmd)
}

func (s *Service) UpdateLastSeenAt(ctx context.Context, cmd *user.UpdateUserLastSeenAtCommand) error {
	return s.store.UpdateLastSeenAt(ctx, cmd)
}

func newSignedInUserCacheKey(orgID, userID int64) string {
	return fmt.Sprintf("signed-in-user-%d-%d", userID, orgID)
}

func (s *Service) Search(ctx context.Context, query *user.SearchUsersQuery) (*user.SearchUserQueryResult, error) {
	return s.store.Search(ctx, query)
}

func (s *Service) Disable(ctx context.Context, cmd *user.DisableUserCommand) error {
	return s.store.Disable(ctx, cmd)
}

func (s *Service) BatchDisableUsers(ctx context.Context, cmd *user.BatchDisableUsersCommand) error {
	return s.store.BatchDisableUsers(ctx, cmd)
}

func (s *Service) UpdatePermissions(ctx context.Context, userID int64, isAdmin bool) error {
	return s.store.UpdatePermissions(ctx, userID, isAdmin)
}

func (s *Service) SetUserHelpFlag(ctx context.Context, cmd *user.SetUserHelpFlagCommand) error {
	return s.store.SetHelpFlag(ctx, cmd)
}

func (s *Service) GetProfile(ctx context.Context, query *user.GetUserProfileQuery) (*user.UserProfileDTO, error) {
	result, err := s.store.GetProfile(ctx, query)
	return result, err
}

// CreateServiceAccount creates a service account in the user table and adds service account to an organisation in the org_user table
func (s *Service) CreateServiceAccount(ctx context.Context, cmd *user.CreateUserCommand) (*user.User, error) {
	cmd.Email = cmd.Login
	err := s.store.LoginConflict(ctx, cmd.Login, cmd.Email, s.caseInsensitiveLogin)
	if err != nil {
		return nil, fmt.Errorf("service account with login %s already exists", cmd.Login)
	}

	// create user
	usr := &user.User{
		Email:            cmd.Email,
		Name:             cmd.Name,
		Login:            cmd.Login,
		IsDisabled:       cmd.IsDisabled,
		Created:          time.Now(),
		Updated:          time.Now(),
		LastSeenAt:       time.Now().AddDate(-10, 0, 0),
		IsServiceAccount: true,
	}

	salt, err := util.GetRandomString(10)
	if err != nil {
		return nil, err
	}
	usr.Salt = salt
	rands, err := util.GetRandomString(10)
	if err != nil {
		return nil, err
	}
	usr.Rands = rands

	_, err = s.store.Insert(ctx, usr)
	if err != nil {
		return nil, err
	}

	return usr, nil
}
