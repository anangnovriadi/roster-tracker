package main

import (
	"context"
	"log/slog"

	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/protobuf/proto"

	storepb "github.com/anangnovriadi/roster-tracker/proto/gen/store"
	"github.com/anangnovriadi/roster-tracker/store"
)

const (
	defaultAdminUsername = "admin"
	defaultAdminPassword = "12345678"
	defaultAdminNickname = "Administrator"
	defaultAdminEmail    = "admin@mail.com"
)

// applyRosterTrackerDefaults enforces the deployment rules of Roster Tracker on
// every start: an admin account exists from the first run, accounts are only
// created by an admin, and anyone may read the instance without signing in.
func applyRosterTrackerDefaults(ctx context.Context, s *store.Store) error {
	if err := ensureDefaultAdmin(ctx, s); err != nil {
		return err
	}

	if _, err := s.UpsertInstanceSetting(ctx, &storepb.InstanceSetting{
		Key: storepb.InstanceSettingKey_ACCESS,
		Value: &storepb.InstanceSetting_AccessSetting{
			AccessSetting: &storepb.InstanceAccessSetting{AccessMode: storepb.InstanceAccessMode_INSTANCE_ACCESS_MODE_PUBLIC},
		},
	}); err != nil {
		return errors.Wrap(err, "failed to enable public access")
	}

	cachedGeneralSetting, err := s.GetInstanceGeneralSetting(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get instance general setting")
	}
	if cachedGeneralSetting.DisallowUserRegistration {
		return nil
	}
	// The cache hands out a shared value; change a copy.
	generalSetting := proto.Clone(cachedGeneralSetting).(*storepb.InstanceGeneralSetting)
	generalSetting.DisallowUserRegistration = true
	if _, err := s.UpsertInstanceSetting(ctx, &storepb.InstanceSetting{
		Key:   storepb.InstanceSettingKey_GENERAL,
		Value: &storepb.InstanceSetting_GeneralSetting{GeneralSetting: generalSetting},
	}); err != nil {
		return errors.Wrap(err, "failed to disable user registration")
	}
	return nil
}

func ensureDefaultAdmin(ctx context.Context, s *store.Store) error {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(defaultAdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return errors.Wrap(err, "failed to hash default admin password")
	}
	_, created, err := s.CreateUserIfNoUsers(ctx, &store.User{
		Username:     defaultAdminUsername,
		Role:         store.RoleAdmin,
		Nickname:     defaultAdminNickname,
		Email:        defaultAdminEmail,
		PasswordHash: string(passwordHash),
	})
	if err != nil {
		return errors.Wrap(err, "failed to create default admin")
	}
	if created {
		slog.Warn("created the default admin account; change its password after signing in", slog.String("username", defaultAdminUsername))
	}
	return nil
}
