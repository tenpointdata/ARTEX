package db

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestUserLifecycle(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		SkipPGUnavailable(t, err)
	}
	defer d.Close()
	name := fmt.Sprintf("u%d", time.Now().UnixNano())

	u, err := d.CreateUser("root-admin", strings.ToUpper(name), "hash-1", RoleReviewer)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = d.Exec(`DELETE FROM artex_users WHERE id = $1`, u.ID) })
	if u.Username != name || u.Role != RoleReviewer || u.Version != 1 || u.Disabled || u.CreatedBy != "root-admin" {
		t.Fatalf("created %+v", u)
	}
	if _, err := d.CreateUser("root-admin", name, "hash-x", RoleAdmin); !errors.Is(err, ErrUserExists) {
		t.Fatalf("duplicate: %v", err)
	}
	if _, err := d.CreateUser("root-admin", "artex", "h", RoleAdmin); err == nil {
		t.Fatal("reserved username accepted by the table")
	}

	a, err := d.GetUserAuth(strings.ToUpper(name))
	if err != nil || a.PasswordHash != "hash-1" || a.ID != u.ID {
		t.Fatalf("auth lookup %+v %v", a, err)
	}
	role := RoleOperator
	u2, err := d.UpdateUser("root-admin", u.ID, UserPatch{Role: &role})
	if err != nil || u2.Role != RoleOperator || u2.Version != 2 {
		t.Fatalf("role update %+v %v", u2, err)
	}
	off := true
	u3, err := d.UpdateUser("root-admin", u.ID, UserPatch{Disabled: &off})
	if err != nil || !u3.Disabled || u3.Version != 3 || u3.Role != RoleOperator {
		t.Fatalf("disable %+v %v", u3, err)
	}
	h := "hash-2"
	if u4, err := d.UpdateUser("root-admin", u.ID, UserPatch{PasswordHash: &h}); err != nil || u4.Version != 4 {
		t.Fatalf("password update %+v %v", u4, err)
	}
	if u5, err := d.UpdateUser("root-admin", u.ID, UserPatch{}); err != nil || u5.Version != 4 {
		t.Fatalf("empty patch bumped the version: %+v %v", u5, err)
	}
	if _, err := d.UpdateUser("root-admin", -1, UserPatch{Role: &role}); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("update missing: %v", err)
	}

	list, err := d.ListUsers()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, x := range list {
		found = found || x.ID == u.ID
	}
	if !found {
		t.Fatal("user missing from list")
	}

	// The audit trigger records the acting username, and never the hash.
	entries, err := d.ListAudit(AuditFilter{Kind: AuditKindRowPrefix + "artex_users", Subject: strconv.FormatInt(u.ID, 10), Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 4 {
		t.Fatalf("audit entries = %d, want >= 4", len(entries))
	}
	for _, e := range entries {
		if e.Actor != "root-admin" {
			t.Errorf("audit actor %q", e.Actor)
		}
		if strings.Contains(string(e.Payload), "hash-") {
			t.Errorf("audit payload carries the hash: %s", e.Payload)
		}
	}

	if err := d.DeleteUser("root-admin", u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.GetUser(u.ID); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("after delete: %v", err)
	}
	if err := d.DeleteUser("root-admin", u.ID); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}
