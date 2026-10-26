package cloudsync

import (
	"context"
	"net/http"
)

// Read fetches an existing space without creating or changing an object. In
// particular, a deleted remote during conflict review is not an empty document.
func (e *Engine) Read(ctx context.Context, config Config, secrets Secrets) (Result, error) {
	config, err := NormalizeConfig(config)
	if err != nil {
		return Result{}, err
	}
	if err = ValidateSecrets(secrets); err != nil {
		return Result{}, err
	}
	encoded, revision, exists, err := e.read(ctx, config, secrets)
	if err != nil {
		return Result{}, err
	}
	if !exists {
		return Result{}, &Error{Code: "remote_missing"}
	}
	identity := config.Endpoint + "\n" + config.Bucket + "\n" + config.Prefix + "/" + objectName
	values, err := openEnvelope(encoded, secrets.Passphrase, identity)
	if err != nil {
		return Result{}, err
	}
	return Result{Values: values, Revision: revision}, nil
}

// Sync never mutates its input maps. A successful result is a proposed local
// commit, including its next baseline. Persist it atomically in the caller.
// The remote object is only replaced if the revision read still exists.
func (e *Engine) Sync(ctx context.Context, config Config, secrets Secrets, local, baseline Values) (Result, error) {
	return e.SyncValidated(ctx, config, secrets, local, baseline, nil)
}

// SyncValidated lets each data owner reject invalid supported fields before
// they can be uploaded or applied locally. Unknown fields remain transportable.
// The callback runs after each merge, including a conditional-write retry.
func (e *Engine) SyncValidated(ctx context.Context, config Config, secrets Secrets, local, baseline Values, validate func(Values) error) (Result, error) {
	config, err := NormalizeConfig(config)
	if err != nil {
		return Result{}, err
	}
	if err := ValidateSecrets(secrets); err != nil {
		return Result{}, err
	}
	if err := validateValues(local); err != nil {
		return Result{}, err
	}
	if err := validateValues(baseline); err != nil {
		return Result{}, err
	}
	identity := config.Endpoint + "\n" + config.Bucket + "\n" + config.Prefix + "/" + objectName
	for attempt := 0; attempt < 3; attempt++ {
		encoded, revision, exists, err := e.read(ctx, config, secrets)
		if err != nil {
			return Result{}, err
		}
		remote := Values{}
		if exists {
			remote, err = openEnvelope(encoded, secrets.Passphrase, identity)
			if err != nil {
				return Result{}, err
			}
		}
		// A deleted remote object is not an instruction to erase every local
		// preference. Recreate it from the current local state under CAS.
		mergeBase := baseline
		if !exists {
			mergeBase = nil
		}
		merged, err := Merge(local, mergeBase, remote)
		if err != nil {
			return Result{}, err
		}
		if validate != nil {
			if err := validate(merged); err != nil {
				return Result{}, err
			}
		}
		if exists && equalValues(merged, remote) {
			return Result{Values: merged, Revision: revision}, nil
		}
		encoded, err = seal(merged, secrets.Passphrase, identity)
		if err != nil {
			return Result{}, err
		}
		res, err := e.request(ctx, http.MethodPut, config, secrets, encoded, revision, !exists)
		if err != nil {
			return Result{}, err
		}
		status, newRevision := res.StatusCode, res.Header.Get("ETag")
		closeResponse(res)
		if status == http.StatusPreconditionFailed || status == http.StatusConflict {
			continue
		}
		if status != http.StatusOK && status != http.StatusCreated && status != http.StatusNoContent {
			return Result{}, responseError(status)
		}
		if !validRevision(newRevision) {
			newRevision = ""
		}
		return Result{Values: merged, Uploaded: true, Revision: newRevision}, nil
	}
	return Result{}, &Error{Code: "concurrent_update"}
}

func equalValues(a, b Values) bool {
	if len(a) != len(b) {
		return false
	}
	for key, av := range a {
		if !equalJSON(av, b[key]) {
			return false
		}
	}
	return true
}
