package main

import (
	"context"
	"errors"
	madmin "github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/pilot"
)

func sixUncreatedUsers(ctx context.Context, c config, group string, tr *postV22VerifyTransport) error {
	admin, err := madmin.NewWithOptions(c.Endpoint, &madmin.Options{Creds: credentials.NewStaticV4(c.RootAccess, c.RootSecret, ""), Transport: tr})
	if err != nil {
		return err
	}
	rows := map[string]any{}
	for _, user := range pilot.RemainingUncreatedIdentities(group) {
		_, err = admin.GetUserInfo(ctx, user)
		var provider madmin.ErrorResponse
		if !errors.As(err, &provider) || provider.Code != "XMinioAdminNoSuchUser" || tr.lastStatus != 404 {
			return errors.New("retained uncreated identity absence unconfirmed")
		}
		rows[user] = map[string]any{"attempt_id": tr.lastID, "status": tr.lastStatus, "code": provider.Code}
	}
	return write(c, "six-uncreated-users.json", map[string]any{"audit_id": tr.auditID, "users": rows})
}
