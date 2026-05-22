package auth

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/apernet/hysteria/core/v2/server"
)

const (
	httpAuthTimeout     = 10 * time.Second
	defaultCacheTTL     = 60 * time.Second
	defaultCacheMaxSize = 16384
	denyCacheTTL        = 5 * time.Second
	maxAllowCacheTTL    = 5 * time.Minute
)

var _ server.Authenticator = &HTTPAuthenticator{}

var errInvalidStatusCode = errors.New("invalid status code")

// cacheEntry stores a cached authentication result.
type cacheEntry struct {
	authID    string
	expiresAt time.Time
}

type HTTPAuthenticator struct {
	Client   *http.Client
	URL      string
	Protocol string // 协议标识，如 "hysteria2"
	NodeID   string // 本节点标识

	allowCache sync.Map // SHA256(credential) → *cacheEntry
	denyCache  sync.Map // SHA256(credential) → time.Time
}

func NewHTTPAuthenticator(url string, insecure bool, protocol, nodeID string) *HTTPAuthenticator {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: insecure,
	}
	return &HTTPAuthenticator{
		Client: &http.Client{
			Transport: tr,
			Timeout:   httpAuthTimeout,
		},
		URL:      url,
		Protocol: protocol,
		NodeID:   nodeID,
	}
}

// httpAuthRequest is the request body sent to the unified auth center.
type httpAuthRequest struct {
	RemoteAddr string `json:"remote_addr"` // 客户端 IP:Port
	Credential string `json:"credential"`  // 用户凭据原文
	Tx         uint64 `json:"tx"`          // 客户端宣称速率
	Protocol   string `json:"protocol"`    // "hysteria2"
	NodeID     string `json:"node_id"`     // 本节点标识
	Timestamp  int64  `json:"timestamp"`   // Unix 秒
}

// httpAuthResponse is the response from the unified auth center.
type httpAuthResponse struct {
	OK  bool   `json:"ok"`
	ID  string `json:"id"`
	Msg string `json:"msg"` // 拒绝原因
	TTL int64  `json:"ttl"` // 缓存秒数，0=不缓存
}

func (a *HTTPAuthenticator) post(req *httpAuthRequest) (*httpAuthResponse, error) {
	bs, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	resp, err := a.Client.Post(a.URL, "application/json", bytes.NewReader(bs))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errInvalidStatusCode
	}
	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var authResp httpAuthResponse
	err = json.Unmarshal(respData, &authResp)
	if err != nil {
		return nil, err
	}
	return &authResp, nil
}

// hashCredential returns the hex-encoded SHA256 hash of the credential.
// Cache keys use the hash so plaintext credentials never sit in memory as map keys.
func hashCredential(credential string) string {
	sum := sha256.Sum256([]byte(credential))
	return fmt.Sprintf("%x", sum)
}

func (a *HTTPAuthenticator) Authenticate(addr net.Addr, auth string, tx uint64) (ok bool, id string) {
	key := hashCredential(auth)

	// 第一关：拒绝缓存（防暴力破解，5s 内不重复请求认证中心）
	if v, hit := a.denyCache.Load(key); hit {
		if time.Now().Before(v.(time.Time)) {
			return false, ""
		}
		a.denyCache.Delete(key)
	}

	// 第二关：放行缓存
	if v, hit := a.allowCache.Load(key); hit {
		entry := v.(*cacheEntry)
		if time.Now().Before(entry.expiresAt) {
			return true, entry.authID
		}
		a.allowCache.Delete(key)
	}

	// 第三关：POST 统一认证中心
	req := &httpAuthRequest{
		RemoteAddr: addr.String(),
		Credential: auth,
		Tx:         tx,
		Protocol:   a.Protocol,
		NodeID:     a.NodeID,
		Timestamp:  time.Now().Unix(),
	}
	resp, err := a.post(req)
	if err != nil || !resp.OK {
		// 写入拒绝缓存
		a.denyCache.Store(key, time.Now().Add(denyCacheTTL))
		return false, ""
	}

	// 写入放行缓存（硬上限 5min）
	ttl := resp.TTL
	if ttl <= 0 {
		ttl = int64(defaultCacheTTL.Seconds())
	}
	if ttl > int64(maxAllowCacheTTL.Seconds()) {
		ttl = int64(maxAllowCacheTTL.Seconds())
	}
	a.allowCache.Store(key, &cacheEntry{
		authID:    resp.ID,
		expiresAt: time.Now().Add(time.Duration(ttl) * time.Second),
	})

	return true, resp.ID
}
