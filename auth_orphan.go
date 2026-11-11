package innpark

import (
	"fmt"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/daos"
	"github.com/pocketbase/pocketbase/tools/types"
)

// orphanEmailHolders returns the other records of the collection that hold
// email although their Firebase identity no longer exists (the account was
// deleted or replaced in Firebase). It runs outside any transaction because it
// calls Firebase. When a record with that email still has a live Firebase
// identity, it returns an error and the caller keeps refusing the creation.
func orphanEmailHolders(dao *daos.Dao, collection, email, newID string, firebaseUserExists func(uid string) (bool, error)) ([]string, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil, nil
	}
	var holders []struct {
		Id string `db:"id"`
	}
	err := dao.DB().Select("id").From(collection).
		Where(dbx.NewExp("lower(email) = lower({:email})", dbx.Params{"email": email})).
		AndWhere(dbx.Not(dbx.HashExp{"id": newID})).
		All(&holders)
	if err != nil {
		return nil, err
	}
	orphans := make([]string, 0, len(holders))
	for _, holder := range holders {
		live, err := firebaseUserExists(holder.Id)
		if err != nil {
			return nil, err
		}
		if live {
			return nil, fmt.Errorf("email already used by a live account %s", holder.Id)
		}
		orphans = append(orphans, holder.Id)
	}
	return orphans, nil
}

// releaseOrphanEmails frees the email of the given orphan records: it becomes
// "deleted+<id>@invalid" and, when the collection has deleted_at, the record
// is soft-deleted if it was not already. Nothing is removed. It starts with a
// write so it can run first inside the transaction that creates the new user.
func releaseOrphanEmails(dao *daos.Dao, collection string, orphanIDs []string) error {
	if len(orphanIDs) == 0 {
		return nil
	}
	now := types.NowDateTime().String()
	for _, id := range orphanIDs {
		if _, err := dao.DB().Update(collection, dbx.Params{
			"email":   "deleted+" + id + "@invalid",
			"updated": now,
		}, dbx.HashExp{"id": id}).Execute(); err != nil {
			return err
		}
	}
	hasDeletedAt := 0
	if err := dao.DB().NewQuery("SELECT COUNT(*) FROM pragma_table_info({:table}) WHERE name = 'deleted_at'").
		Bind(dbx.Params{"table": collection}).Row(&hasDeletedAt); err != nil {
		return err
	}
	if hasDeletedAt == 0 {
		return nil
	}
	for _, id := range orphanIDs {
		if _, err := dao.DB().Update(collection, dbx.Params{"deleted_at": now},
			dbx.And(dbx.HashExp{"id": id}, dbx.NewExp("COALESCE(deleted_at, '') = ''")),
		).Execute(); err != nil {
			return err
		}
	}
	return nil
}
