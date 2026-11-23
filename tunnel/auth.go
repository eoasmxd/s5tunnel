package tunnel

import (
	"crypto/md5"
	"crypto/subtle"
	"encoding/hex"
	"strconv"
	"time"
)

const authTimeWindow = 300

func GetAuthKey(token string, timestamp int64) string {
	h := md5.Sum([]byte(token + strconv.FormatInt(timestamp, 10)))
	return hex.EncodeToString(h[:])
}

func CheckAuth(token, timestampStr, authKey string) bool {
	if token == "" {
		return true
	}
	if authKey == "" || timestampStr == "" {
		return false
	}
	ts, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return false
	}
	diff := time.Now().Unix() - ts
	if diff < -authTimeWindow || diff > authTimeWindow {
		return false
	}
	expected := GetAuthKey(token, ts)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(authKey)) == 1
}

