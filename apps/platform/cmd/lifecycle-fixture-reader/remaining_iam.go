package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	madmin "github.com/minio/madmin-go/v3"
	"github.com/qq550723504/data-product-platform/apps/platform/internal/lifecycle/domain"
)

func remainingPolicies(cs []domain.Copy) ([]byte, []byte, error) {
	var reads []any
	for _, cp := range cs {
		if cp.Key == "fixture/control-hold.csv" || cp.Key == "fixture/control-retention.csv" {
			continue
		}
		reads = append(reads, map[string]any{"Effect": "Allow", "Action": []string{"s3:GetObjectVersion"}, "Resource": []string{"arn:aws:s3:::" + cp.Namespace + "/" + cp.Key}, "Condition": map[string]any{"StringEquals": map[string]string{"s3:versionid": cp.ProviderVersion}}})
	}
	if len(reads) == 0 {
		return nil, nil, errors.New("no fixed read scope")
	}
	read, _ := json.Marshal(map[string]any{"Version": "2012-10-17", "Statement": reads})
	put, _ := json.Marshal(map[string]any{"Version": "2012-10-17", "Statement": []any{map[string]any{"Effect": "Allow", "Action": []string{"s3:PutObject"}, "Resource": []string{"arn:aws:s3:::" + namespaces[0] + "/fixture/remaining-standardized.csv", "arn:aws:s3:::" + namespaces[0] + "/fixture/remaining-processed.csv"}}}})
	return read, put, nil
}
func remainingBindOutputs(ctx context.Context, r remainingConfig, original []domain.Copy) error {
	c := r.Base
	if r.Group != "business-races" || c.RootAccess == "" || c.RootSecret == "" {
		return errors.New("separate output policy admin required")
	}
	pool, err := pgxpool.New(ctx, c.PostgresDSN)
	if err != nil {
		return err
	}
	defer pool.Close()
	all, err := planeCopies(ctx, c, pool)
	if err != nil {
		return err
	}
	originalIDs := map[string]bool{}
	for _, cp := range original {
		originalIDs[cp.ID.String()] = true
	}
	extra := 0
	for _, cp := range all {
		if originalIDs[cp.ID.String()] {
			continue
		}
		if cp.Key != "fixture/remaining-standardized.csv" && cp.Key != "fixture/remaining-processed.csv" {
			return errors.New("unexpected output membership")
		}
		var ready bool
		if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM lifecycle_processing_output_ready WHERE copy_id=$1)`, cp.ID).Scan(&ready); err != nil || !ready {
			return errors.New("output lacks immutable ready fact")
		}
		extra++
	}
	if len(all) != len(original)+extra || extra < 1 || extra > 2 {
		return errors.New("fixed output membership required")
	}
	p, _, err := remainingPolicies(all)
	if err != nil {
		return err
	}
	a, err := madmin.New(c.Endpoint, c.RootAccess, c.RootSecret, false)
	if err != nil {
		return err
	}
	a.SetCustomTransport(adminTransport{&http.Transport{Proxy: nil, DisableKeepAlives: true, ResponseHeaderTimeout: 10 * time.Second}, c})
	x, cancel := context.WithTimeout(context.WithValue(ctx, adminKey{}, &adminCall{}), 15*time.Second)
	defer cancel()
	if err = a.AddCannedPolicy(x, "lifecycle307-remaining-business-"+c.Access, p); err != nil {
		return err
	}
	return write(c, "remaining-reader-output-scope-"+time.Now().UTC().Format("150405.000000000")+".json", map[string]any{"policy": json.RawMessage(p), "copies": all, "new_survivors": extra, "delete": 0})
}
func remainingProvision(ctx context.Context, r remainingConfig, cs []domain.Copy, disable bool) error {
	c := r.Base
	if r.Group != "business-races" || c.RootAccess == "" || c.RootSecret == "" || c.ProducerAccess == "" || c.ProducerSecret == "" {
		return errors.New("separate fixed admin composition required")
	}
	a, err := madmin.New(c.Endpoint, c.RootAccess, c.RootSecret, false)
	if err != nil {
		return err
	}
	a.SetCustomTransport(adminTransport{&http.Transport{Proxy: nil, DisableKeepAlives: true, ResponseHeaderTimeout: 10 * time.Second}, c})
	call := func(fn func(context.Context) error) error {
		x, cancel := context.WithTimeout(context.WithValue(ctx, adminKey{}, &adminCall{}), 15*time.Second)
		defer cancel()
		return fn(x)
	}
	if disable {
		for _, user := range []string{c.Access, c.ProducerAccess, c.InspectAccess} {
			if err = call(func(x context.Context) error { return a.SetUserStatus(x, user, madmin.AccountDisabled) }); err != nil {
				return err
			}
		}
		return write(c, "remaining-identities-disabled.json", map[string]any{"reader": c.Access, "producer": c.ProducerAccess, "status": "disabled", "delete": 0})
	}
	read, put, err := remainingPolicies(cs)
	if err != nil {
		return err
	}
	for i, p := range [][]byte{read, put} {
		access, secret := c.Access, c.Secret
		if i == 1 {
			access, secret = c.ProducerAccess, c.ProducerSecret
		}
		name := "lifecycle307-remaining-business-" + access
		if err = call(func(x context.Context) error { return a.AddCannedPolicy(x, name, p) }); err != nil {
			return err
		}
		if err = call(func(x context.Context) error { return a.AddUser(x, access, secret) }); err != nil {
			return err
		}
		if err = call(func(x context.Context) error {
			_, e := a.AttachPolicy(x, madmin.PolicyAssociationReq{Policies: []string{name}, User: access})
			return e
		}); err != nil {
			return err
		}
		if err = write(c, "remaining-policy-"+access+".json", json.RawMessage(p)); err != nil {
			return err
		}
	}
	if err = call(func(x context.Context) error { return a.SetUserStatus(x, c.InspectAccess, madmin.AccountEnabled) }); err != nil {
		return err
	}
	return nil
}
