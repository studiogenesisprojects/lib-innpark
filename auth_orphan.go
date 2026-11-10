package innpark

import (
	"fmt"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/daos"
	"github.com/pocketbase/pocketbase/tools/types"
)

// releaseOrphanEmail frees email for a new account when other records of the
// collection still hold it but their Firebase identity no longer exists (the
// account was deleted or replaced in Firebase). Their email becomes
// "deleted+<id>@invalid"; nothing else is changed or removed. When another
// record with that email still has a live Firebase identity, it returns an
// error and the caller keeps refusing the creation.
func releaseOrphanEmail(dao *daos.Dao, collection, email, newID string, firebaseUserExists func(uid string) (bool, error)) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil
	}
	var holders []struct {
		Id string `db:"id"`
	}
	err := dao.DB().Select("id").From(collection).
		Where(dbx.NewExp("lower(email) = lower({:email})", dbx.Params{"email": email})).
		AndWhere(dbx.Not(dbx.HashExp{"id": newID})).
		All(&holders)
	if err != nil {
		return err
	}
	for _, holder := range holders {
		live, err := firebaseUserExists(holder.Id)
		if err != nil {
			return err
		}
		if live {
			return fmt.Errorf("email already used by a live account %s", holder.Id)
		}
		if _, err := dao.DB().Update(collection, dbx.Params{
			"email":   "deleted+" + holder.Id + "@invalid",
			"updated": types.NowDateTime().String(),
		}, dbx.HashExp{"id": holder.Id}).Execute(); err != nil {
			return err
		}
	}
	return nil
}
