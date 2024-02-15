package health

import (
	"errors"
	"net/http"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
)

var (
	errNotSynced = errors.New("not synced")
)

func checkSynced(ec *ethclient.Client, r *http.Request) error {
	i, err := ec.SyncProgress(r.Context())
	if err != nil {
		log.Root().Warn("Unable to check sync status for healthcheck", "err", err.Error())
		return err
	}
	if i == nil {
		return nil
	}

	return errNotSynced
}
