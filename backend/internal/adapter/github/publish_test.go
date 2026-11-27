package github

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Avtobot22/craftPulse/backend/pkg/adaptersdk"
)

func TestPublicationLockSerializesSameOrganizationAndPull(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		a := &Adapter{}
		key := publicationKey{org: "first", pullID: 1}
		release, err := a.lockPublication(t.Context(), key)
		if err != nil {
			t.Fatal(err)
		}

		otherKey := publicationKey{org: "second", pullID: key.pullID}
		otherRelease, err := a.lockPublication(t.Context(), otherKey)
		if err != nil {
			t.Fatal(err)
		}
		otherRelease()

		expired := make(chan error, 1)
		go func() {
			ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
			defer cancel()
			unlock, err := a.lockPublication(ctx, key)
			if unlock != nil {
				unlock()
			}
			expired <- err
		}()
		if err := <-expired; !hasCode(err, adaptersdk.AdapterErrorCodeTimeout) {
			t.Errorf("blocked caller got %v, want timeout", err)
		}

		acquired := make(chan error, 1)
		go func() {
			unlock, err := a.lockPublication(t.Context(), key)
			if unlock != nil {
				unlock()
			}
			acquired <- err
		}()
		synctest.Wait()
		if len(acquired) != 0 {
			t.Error("same PR entered publication while the lock was held")
		}
		release()
		if err := <-acquired; err != nil {
			t.Fatal(err)
		}
		if len(a.publications) != 0 {
			t.Error("publication lock was not removed")
		}
	})
}
