package health

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/ledgerwatch/erigon/rpc"
	"github.com/ledgerwatch/log/v3"
	"github.com/tendermint/tendermint/types/time"
)

type requestBody struct {
	MinPeerCount *uint            `json:"min_peer_count"`
	BlockNumber  *rpc.BlockNumber `json:"known_block"`
}

const (
	urlPath      = "/health"
	healthHeader = "X-ERIGON-HEALTHCHECK"
)

var (
	errCheckDisabled = errors.New("error check disabled")
)

func ProcessHealthcheckIfNeeded(
	w http.ResponseWriter,
	r *http.Request,
	rpcAPI []rpc.API,
) bool {
	if !strings.EqualFold(r.URL.Path, urlPath) {
		return false
	}

	header := r.Header.Get(healthHeader)
	if header != "" {
		err := ProcessHealthcheck2(w, r, rpcAPI)
		if err != nil {
			// health check failed if error
			w.WriteHeader(http.StatusInternalServerError)
		}
		w.Write([]byte(errorStringOrOK(err)))

		return true
	}

	netAPI, ethAPI := parseAPI(rpcAPI)

	var errMinPeerCount = errCheckDisabled
	var errCheckBlock = errCheckDisabled

	body, errParse := parseHealthCheckBody(r.Body)
	defer r.Body.Close()

	if errParse != nil {
		log.Root().Warn("unable to process healthcheck request", "err", errParse)
	} else {
		// 1. net_peerCount
		if body.MinPeerCount != nil {
			errMinPeerCount = checkMinPeers(*body.MinPeerCount, netAPI)
		}
		// 2. custom query (shouldn't fail)
		if body.BlockNumber != nil {
			errCheckBlock = checkBlockNumber(*body.BlockNumber, ethAPI)
		}
		// TODO add time from the last sync cycle
	}

	err := reportHealth(errParse, errMinPeerCount, errCheckBlock, w)
	if err != nil {
		log.Root().Warn("unable to process healthcheck request", "err", err)
	}

	return true
}

func ProcessHealthcheck2(
	w http.ResponseWriter,
	r *http.Request,
	rpcAPI []rpc.API) error {
	netAPI, ethAPI := parseAPI(rpcAPI)
	headers := r.Header.Values(healthHeader)
	for _, header := range headers {
		header = strings.ToLower(header)
		if header == "synced" {
			err := processSyncedCheck(w, r, rpcAPI)
			if err != nil {
				return err
			}
		}
		if strings.HasPrefix(header, "check_block") {
			blockNumber, err := strconv.Atoi(strings.TrimPrefix(header, "check_block"))
			if err != nil {
				return err
			}
			err = checkBlockNumber(rpc.BlockNumber(blockNumber), ethAPI)
			if err != nil {
				return err
			}
		}
		if strings.HasPrefix(header, "min_peer_count") {
			minPeers, err := strconv.Atoi(strings.TrimPrefix(header, "min_peer_count"))
			if err != nil {
				return err
			}
			err = checkMinPeers(uint(minPeers), netAPI)
			if err != nil {
				return err
			}
		}
		if strings.HasPrefix(header, "max_seconds_behind") {
			secs, err := strconv.Atoi(strings.TrimPrefix(header, "max_seconds_behind"))
			if err != nil {
				return err
			}
			if secs < 0 {
				secs = 600 // a somewhat sane default value
			}
			now := time.Now().Unix()
			err = processTimeCheck(r, int(now)-secs, rpcAPI)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func processSyncedCheck(
	w http.ResponseWriter,
	r *http.Request,
	rpcAPI []rpc.API,
) error {
	_, ethAPI := parseAPI(rpcAPI)

	i, err := ethAPI.Syncing(r.Context())
	if err != nil {
		log.Root().Warn("unable to process synced request", "err", err.Error())
		return err
	}
	if i == nil || i == false {
		return nil
	}
	return errors.New("not synced")
}

func processTimeCheck(
	r *http.Request,
	seconds int,
	rpcAPI []rpc.API,
) error {
	_, ethAPI := parseAPI(rpcAPI)

	i, err := ethAPI.GetBlockByNumber(r.Context(), rpc.LatestBlockNumber, false)
	if err != nil {
		return err
	}
	timestamp := 0
	if ts, ok := i["timestamp"]; ok {
		if cs, ok := ts.(uint64); ok {
			timestamp = int(cs)
		}
	}
	if timestamp > seconds {
		return fmt.Errorf("got ts: %d, need: %d", timestamp, seconds)
	}

	return nil
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

func reportHealth(errParse, errMinPeerCount, errCheckBlock error, w http.ResponseWriter) error {
	statusCode := http.StatusOK
	errors := make(map[string]string)

	if shouldChangeStatusCode(errParse) {
		statusCode = http.StatusInternalServerError
	}
	errors["healthcheck_query"] = errorStringOrOK(errParse)

	if shouldChangeStatusCode(errMinPeerCount) {
		statusCode = http.StatusInternalServerError
	}
	errors["min_peer_count"] = errorStringOrOK(errMinPeerCount)

	if shouldChangeStatusCode(errCheckBlock) {
		statusCode = http.StatusInternalServerError
	}
	errors["check_block"] = errorStringOrOK(errCheckBlock)

	w.WriteHeader(statusCode)

	bodyJson, err := json.Marshal(errors)
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
