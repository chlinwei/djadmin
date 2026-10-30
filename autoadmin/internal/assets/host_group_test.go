package assets

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"testing"

	db "autoadmin/internal/platform/database/generated"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestHostGroupOptionsAreFlatWithDirectCounts(t *testing.T) {
	rows := []db.ListAllHostGroupsRow{
		{ID: 1, Name: "production", HostCount: 2},
		{ID: 2, Name: "testing", HostCount: 3},
	}
	groups := buildHostGroupOptions(rows)
	if len(groups) != 2 || groups[0].ID != 1 || groups[0].HostCount != 2 ||
		groups[1].ID != 2 || groups[1].HostCount != 3 {
		t.Fatalf("expected all groups with their own counts: %#v", groups)
	}
	payload, err := json.Marshal(groups)
	if err != nil {
		t.Fatal(err)
	}
	var items []map[string]any
	if err := json.Unmarshal(payload, &items); err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		for _, field := range []string{"parent", "parent_id", "parent_name", "children"} {
			if _, found := item[field]; found {
				t.Fatalf("flat group must not expose %s: %s", field, payload)
			}
		}
	}
	empty, err := json.Marshal(buildHostGroupOptions(nil))
	if err != nil || string(empty) != "[]" {
		t.Fatalf("empty options must be an array: %s, %v", empty, err)
	}
}

func TestHostGroupRejectsParentBeforeDatabaseAccess(t *testing.T) {
	service := &Service{}
	for _, parent := range []int64{0, 1, -1} {
		input := HostGroupInput{Name: "group", ParentID: &parent}
		if _, err := service.CreateHostGroup(context.Background(), input); !errors.Is(err, ErrGroupParent) {
			t.Fatalf("create must reject parent %d: %v", parent, err)
		}
		if _, err := service.UpdateHostGroup(context.Background(), 7, input); !errors.Is(err, ErrGroupParent) {
			t.Fatalf("update must reject parent %d: %v", parent, err)
		}
	}
}

func TestHostGroupDeletionProtectsHosts(t *testing.T) {
	for _, hosts := range []int64{0, 3} {
		t.Run(strconv.FormatInt(hosts, 10), func(t *testing.T) {
			pool, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			mock.ExpectQuery(`SELECT COUNT\(\*\) FROM assets_host WHERE group_id =`).
				WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(hosts))
			if hosts == 0 {
				mock.ExpectExec("DELETE FROM assets_hostgroup WHERE id =").
					WithArgs(int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			err = NewRepository(pool).DeleteHostGroup(context.Background(), 7)
			if hosts > 0 && !errors.Is(err, ErrDeleteProtected) || hosts == 0 && err != nil {
				t.Fatalf("unexpected deletion result for %d hosts: %v", hosts, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
