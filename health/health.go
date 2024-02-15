package health

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/log"
)

const (
	healthHeader     = "X-GETH-HEALTHCHECK"
	synced           = "synced"
	minPeerCount     = "min_peer_count"
	checkBlock       = "check_block"
	maxSecondsBehind = "max_seconds_behind"
)

var (
	errCheckDisabled  = errors.New("error check disabled")
	errBadHeaderValue = errors.New("bad header value")
)

type requestBody struct {
	Synced           *bool   `json:"synced"`
	MinPeerCount     *uint   `json:"min_peer_count"`
	CheckBlock       *uint64 `json:"check_block"`
	MaxSecondsBehind *int    `json:"max_seconds_behind"`
}

func (h *handler) processFromHeaders(headers []string, w http.ResponseWriter, r *http.Request) {
	var (
		errCheckSynced  = errCheckDisabled
		errCheckPeer    = errCheckDisabled
		errCheckBlock   = errCheckDisabled
		errCheckSeconds = errCheckDisabled
	)

	for _, header := range headers {
		lHeader := strings.ToLower(header)
		if lHeader == synced {
			errCheckSynced = checkSynced(h.ec, r)
		}
		if strings.HasPrefix(lHeader, minPeerCount) {
			peers, err := strconv.Atoi(strings.TrimPrefix(lHeader, minPeerCount))
			if err != nil {
				errCheckPeer = err
				break
			}
			errCheckPeer = checkMinPeers(h.ec, uint(peers))
		}
		if strings.HasPrefix(lHeader, checkBlock) {
			block, err := strconv.Atoi(strings.TrimPrefix(lHeader, checkBlock))
			if err != nil {
				errCheckBlock = err
				break
			}
			errCheckBlock = checkBlockNumber(h.ec, big.NewInt(int64(block)))
		}
		if strings.HasPrefix(lHeader, maxSecondsBehind) {
			seconds, err := strconv.Atoi(strings.TrimPrefix(lHeader, maxSecondsBehind))
			if err != nil {
				errCheckSeconds = err
				break
			}
			if seconds < 0 {
				errCheckSeconds = errBadHeaderValue
				break
			}
			now := time.Now().Unix()
			errCheckSeconds = checkTime(h.ec, r, int(now)-seconds)
		}
	}

	reportHealth(errCheckSynced, errCheckPeer, errCheckBlock, errCheckSeconds, w)
}

func (h *handler) processFromBody(w http.ResponseWriter, r *http.Request) {
	body, errParse := parseHealthCheckBody(r.Body)
	defer r.Body.Close()

	var (
		errCheckSynced  = errCheckDisabled
		errCheckPeer    = errCheckDisabled
		errCheckBlock   = errCheckDisabled
		errCheckSeconds = errCheckDisabled
	)

	if errParse != nil {
		log.Root().Warn("Unable to process healthcheck request", "err", errParse)
	} else {
		if body.Synced != nil {
			errCheckSynced = checkSynced(h.ec, r)
		}

		if body.MinPeerCount != nil {
			errCheckPeer = checkMinPeers(h.ec, *body.MinPeerCount)
		}

		if body.CheckBlock != nil {
			errCheckBlock = checkBlockNumber(h.ec, big.NewInt(int64(*body.CheckBlock)))
		}

		if body.MaxSecondsBehind != nil {
			seconds := *body.MaxSecondsBehind
			if seconds < 0 {
				errCheckSeconds = errBadHeaderValue
			}
			now := time.Now().Unix()
			errCheckSeconds = checkTime(h.ec, r, int(now)-seconds)
		}
	}

	err := reportHealth(errCheckSynced, errCheckPeer, errCheckBlock, errCheckSeconds, w)
	if err != nil {
		log.Root().Warn("Unable to process healthcheck request", "err", err)
	}
}

func reportHealth(errCheckSynced, errCheckPeer, errCheckBlock, errCheckSeconds error, w http.ResponseWriter) error {
	statusCode := http.StatusOK
	errs := make(map[string]string)

	if shouldChangeStatusCode(errCheckSynced) {
		statusCode = http.StatusInternalServerError
	}
	errs[synced] = errorStringOrOK(errCheckSynced)

	if shouldChangeStatusCode(errCheckPeer) {
		statusCode = http.StatusInternalServerError
	}
	errs[minPeerCount] = errorStringOrOK(errCheckPeer)

	if shouldChangeStatusCode(errCheckBlock) {
		statusCode = http.StatusInternalServerError
	}
	errs[checkBlock] = errorStringOrOK(errCheckBlock)

	if shouldChangeStatusCode(errCheckSeconds) {
		statusCode = http.StatusInternalServerError
	}
	errs[maxSecondsBehind] = errorStringOrOK(errCheckSeconds)

	return writeResponse(w, errs, statusCode)
}

func parseHealthCheckBody(reader io.Reader) (requestBody, error) {
	var body requestBody

	bodyBytes, err := io.ReadAll(reader)
	if err != nil {
		return body, err
	}

	err = json.Unmarshal(bodyBytes, &body)
	if err != nil {
		return body, err
	}

	return body, nil
}

func writeResponse(w http.ResponseWriter, errs map[string]string, statusCode int) error {
	w.WriteHeader(statusCode)

	bodyJson, err := json.Marshal(errs)
	if err != nil {
		return err
	}

	_, err = w.Write(bodyJson)
	if err != nil {
		return err
	}

	return nil
}

func shouldChangeStatusCode(err error) bool {
	return err != nil && !errors.Is(err, errCheckDisabled)
}

func errorStringOrOK(err error) string {
	if err == nil {
		return "HEALTHY"
	}

	if errors.Is(err, errCheckDisabled) {
		return "DISABLED"
	}

	return fmt.Sprintf("ERROR: %v", err)
}
