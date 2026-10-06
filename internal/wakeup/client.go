package wakeup

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// maxResponseBytes caps how much of a WakeUp response is read.
const maxResponseBytes = 4 << 20

// processStart gives a monotonic millisecond clock for the anti-replay field.
var processStart = time.Now()

func monotonicMillis() int64 {
	return time.Since(processStart).Milliseconds()
}

// Options configures a Client. Zero values fall back to the baked APK facts and
// a default Android ID.
type Options struct {
	Host      string
	AndroidID string
	CUID      string
	ADID      string

	PublicToken  string
	VersionCode  int
	VersionName  string
	Package      string
	Channel      string
	SignatureMD5 string

	SDK          int
	Device       string
	Brand        string
	Screensize   string
	ABIs         string
	AppBit       string
	DeviceID     string
	OperatorID   string
	DownloadType string
	AppID        string
	Province     string
	City         string
	Area         string

	HTTPClient *http.Client
	Timeout    time.Duration
}

// Client calls the WakeUp share endpoints.
type Client struct {
	host         string
	cuid         string
	adid         string
	publicToken  string
	packageName  string
	versionCode  int
	versionName  string
	channel      string
	signatureMD5 string

	sdk          int
	device       string
	brand        string
	screensize   string
	abis         string
	appBit       string
	deviceID     string
	operatorID   string
	downloadType string
	appID        string
	province     string
	city         string
	area         string

	httpClient *http.Client
	timeout    time.Duration
}

// NewClient applies defaults and returns a ready client.
func NewClient(opts Options) *Client {
	androidID := opts.AndroidID
	if androidID == "" && opts.CUID == "" {
		androidID = DefaultAndroidID
	}
	cuid := opts.CUID
	if cuid == "" {
		cuid = CUIDFromAndroidID(androidID)
	}
	adid := opts.ADID
	// Only derive adid from a real Android ID. A caller that pins CUID without
	// an Android ID would otherwise get an adid derived from the empty string.
	if adid == "" && androidID != "" {
		adid = ADIDFromAndroidID(androidID)
	}

	client := &Client{
		host:         firstNonEmpty(opts.Host, DefaultHost),
		cuid:         cuid,
		adid:         adid,
		publicToken:  firstNonEmpty(opts.PublicToken, APKPublicToken),
		packageName:  firstNonEmpty(opts.Package, APKPackage),
		versionCode:  firstNonZero(opts.VersionCode, APKVersionCode),
		versionName:  firstNonEmpty(opts.VersionName, APKVersionName),
		channel:      firstNonEmpty(opts.Channel, APKChannel),
		signatureMD5: firstNonEmpty(opts.SignatureMD5, APKSignatureMD5),
		sdk:          firstNonZero(opts.SDK, 35),
		device:       firstNonEmpty(opts.Device, "Pixel 7"),
		brand:        firstNonEmpty(opts.Brand, "google"),
		screensize:   firstNonEmpty(opts.Screensize, "1080x2400"),
		abis:         firstNonEmpty(opts.ABIs, "arm64-v8a"),
		appBit:       firstNonEmpty(opts.AppBit, "64"),
		deviceID:     opts.DeviceID,
		operatorID:   opts.OperatorID,
		downloadType: firstNonEmpty(opts.DownloadType, "1"),
		appID:        firstNonEmpty(opts.AppID, "wakeup"),
		province:     opts.Province,
		city:         opts.City,
		area:         opts.Area,
		httpClient:   opts.HTTPClient,
		timeout:      opts.Timeout,
	}
	if client.httpClient == nil {
		// The app reuses one session for the antispam and share requests, so
		// keep cookies from the first response for the second one.
		jar, _ := cookiejar.New(nil)
		client.httpClient = &http.Client{Jar: jar}
	}
	if client.timeout <= 0 {
		client.timeout = DefaultTimeout
	}
	return client
}

// FetchShareData runs the full antispam + share flow and returns the decrypted
// "shareData" text. An empty string with no error means the code was rejected
// or has expired.
func (c *Client) FetchShareData(ctx context.Context, code string) (string, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return "", fmt.Errorf("分享口令为空")
	}
	rand10, err := generateRand10()
	if err != nil {
		return "", err
	}
	signA, err := c.makeSignA(rand10)
	if err != nil {
		return "", err
	}
	common := c.commonParams()

	signB, err := c.requestSignB(ctx, signA, common)
	if err != nil {
		return "", err
	}
	token, err := tokenFromSignB(signB, rand10)
	if err != nil {
		return "", err
	}

	rc4Key := nativeGetKey(strconv.Itoa(c.versionCode), token)
	encryptedParams, err := rc4Crypt([]byte("key="+url.QueryEscape(code)), []byte(rc4Key))
	if err != nil {
		return "", err
	}
	dataValue := base64.StdEncoding.EncodeToString(encryptedParams)

	serverTime := time.Now().Unix()
	kakorr := monotonicMillis()
	signList := make([]string, 0, len(common)+5)
	signList = append(signList, "data="+dataValue)
	for _, p := range common {
		signList = append(signList, p.name+"="+p.value)
	}
	signList = append(signList,
		"nt=wifi",
		fmt.Sprintf("_t_=%d", serverTime),
		fmt.Sprintf("kakorrhaphiophobia=%d", kakorr),
	)
	sort.Strings(signList)
	base64Param := base64.StdEncoding.EncodeToString([]byte(strings.Join(signList, "")))
	sign := nativeGetSign(base64Param, token)

	shareParams := make([]param, 0, len(common)+2)
	shareParams = append(shareParams, param{"data", dataValue})
	shareParams = append(shareParams, common...)
	shareParams = append(shareParams, param{"nt", "wifi"})
	body := "&" + formEncode(shareParams) +
		fmt.Sprintf("&sign=%s&_t_=%d&kakorrhaphiophobia=%d", sign, serverTime, kakorr)

	responseBody, err := c.postForm(ctx, sharePath, body)
	if err != nil {
		return "", err
	}
	payload, err := decodeEnvelope(responseBody)
	if err != nil {
		return "", err
	}
	if err := envelopeError(payload); err != nil {
		return "", err
	}
	encrypted, ok := firstString(payload, "data.data", "data", "result.data")
	if !ok {
		return "", fmt.Errorf("WakeUp 响应缺少密文")
	}
	cipher, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", fmt.Errorf("WakeUp 密文不是合法 base64: %w", err)
	}
	plain, err := rc4Crypt(cipher, []byte(rc4Key))
	if err != nil {
		return "", err
	}
	var decoded struct {
		ShareData string `json:"shareData"`
	}
	if err := json.Unmarshal(plain, &decoded); err != nil {
		return "", fmt.Errorf("WakeUp 解密结果不是 JSON: %w", err)
	}
	return decoded.ShareData, nil
}

// makeSignA builds the encrypted anti-spam probe for one rand10 nonce.
func (c *Client) makeSignA(rand10 string) (string, error) {
	plain := fmt.Sprintf("%s##%s##%s##%s", magic, rand10, c.signatureMD5, c.cuid)
	cipher, err := nativeDESEncrypt([]byte(plain), []byte(signAKey))
	if err != nil {
		return "", err
	}
	return nativeHexEncode(cipher), nil
}

// requestSignB posts signA and returns the server's signB.
func (c *Client) requestSignB(ctx context.Context, signA string, common []param) (string, error) {
	params := make([]param, 0, len(common)+1)
	params = append(params, param{"data", signA})
	params = append(params, common...)
	responseBody, err := c.postForm(ctx, antispamPath, formEncode(params)+"&")
	if err != nil {
		return "", err
	}
	payload, err := decodeEnvelope(responseBody)
	if err != nil {
		return "", err
	}
	if err := envelopeError(payload); err != nil {
		return "", err
	}
	signB, ok := firstString(payload, "data.data", "data", "result.data")
	if !ok {
		return "", fmt.Errorf("WakeUp 反垃圾响应缺少 signB")
	}
	return signB, nil
}

func (c *Client) postForm(ctx context.Context, path, body string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.host, "/")+path, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("na__zyb_source__", "wakeup")
	if c.cuid != "" {
		req.Header.Set("zyb-cuid", c.cuid)
	}
	if c.adid != "" {
		req.Header.Set("zyb-adid", c.adid)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 WakeUp 失败: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("读取 WakeUp 响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("WakeUp 返回 HTTP %d", resp.StatusCode)
	}
	return data, nil
}

type param struct {
	name  string
	value string
}

func (c *Client) commonParams() []param {
	return []param{
		{"area", c.area},
		{"screensize", c.screensize},
		{"cuid", c.cuid},
		{"os", "android"},
		{"city", c.city},
		{"abis", c.abis},
		{"channel", c.channel},
		{"appBit", c.appBit},
		{"vc", strconv.Itoa(c.versionCode)},
		{"deviceId", c.deviceID},
		{"token", c.publicToken},
		{"adid", c.adid},
		{"province", c.province},
		{"pkgName", c.packageName},
		{"appId", c.appID},
		{"download_type", c.downloadType},
		{"vcname", c.versionName},
		{"sdk", strconv.Itoa(c.sdk)},
		{"device", c.device},
		{"brand", c.brand},
		{"operatorid", c.operatorID},
	}
}

func formEncode(params []param) string {
	parts := make([]string, 0, len(params))
	for _, p := range params {
		parts = append(parts, p.name+"="+url.QueryEscape(p.value))
	}
	return strings.Join(parts, "&")
}

// decodeEnvelope parses a JSON response into a generic map.
func decodeEnvelope(body []byte) (map[string]any, error) {
	var payload map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(body), &payload); err != nil {
		return nil, fmt.Errorf("WakeUp 响应不是 JSON: %w", err)
	}
	return payload, nil
}

// envelopeError reports a non-zero errNo on a WakeUp response envelope. A
// missing or non-numeric errNo is treated as success.
func envelopeError(payload map[string]any) error {
	errNo, ok := payload["errNo"].(float64)
	if !ok || errNo == 0 {
		return nil
	}
	return fmt.Errorf("WakeUp 接口返回错误 %v: %v", payload["errNo"], payload["errstr"])
}

// firstString walks dotted paths and returns the first non-empty string found.
func firstString(payload map[string]any, paths ...string) (string, bool) {
	for _, path := range paths {
		var current any = payload
		found := true
		for _, key := range strings.Split(path, ".") {
			obj, ok := current.(map[string]any)
			if !ok {
				found = false
				break
			}
			current, ok = obj[key]
			if !ok {
				found = false
				break
			}
		}
		if !found {
			continue
		}
		if value, ok := current.(string); ok && value != "" {
			return value, true
		}
	}
	return "", false
}

// CUIDFromAndroidID derives the WakeUp cuid from an Android ID.
func CUIDFromAndroidID(androidID string) string {
	return strings.ToUpper(md5Hex("com.baidu"+androidID)) + "|0"
}

// ADIDFromAndroidID derives the WakeUp adid (not the Google advertising id).
func ADIDFromAndroidID(androidID string) string {
	prefix := md5Hex("alpha.beta" + androidID)
	return prefix + adidChecksum(prefix)
}

func adidChecksum(md5Text string) string {
	if len(md5Text) != 32 {
		return "00000000"
	}
	high, errHigh := strconv.ParseUint(md5Text[:16], 16, 64)
	low, errLow := strconv.ParseUint(md5Text[16:], 16, 64)
	if errHigh != nil || errLow != nil {
		return "00000000"
	}
	folded := high ^ low
	return fmt.Sprintf("%08x", uint32(folded>>32)^uint32(folded))
}

// tokenFromSignB decrypts signB with the rand10-derived key and extracts the
// 10-byte anti-spam token.
func tokenFromSignB(signB, rand10 string) (string, error) {
	if len(rand10) < 5 {
		return "", fmt.Errorf("rand10 长度不足")
	}
	cipher, err := nativeHexDecode(signB)
	if err != nil {
		return "", err
	}
	plain, err := nativeDESDecrypt(cipher, []byte(rand10[:5]+"#G4"))
	if err != nil {
		return "", err
	}
	if len(plain) < 22 {
		return "", fmt.Errorf("signB 明文长度异常: %d", len(plain))
	}
	if string(plain[:10]) != rand10 {
		return "", fmt.Errorf("signB 随机串不匹配")
	}
	return string(plain[12:22]), nil
}

// nativeGetKey derives the RC4 key from the APK version and anti-spam token.
func nativeGetKey(typeStr, token string) string {
	m0 := md5Hex(keySalt)
	m1 := md5Hex(typeStr)
	m2 := md5Hex("[" + token + "]@")
	m2 = reverseASCII(m2[17:32]) + m2[15:17] + reverseASCII(m2[0:15])
	chars := []byte(m0 + m1 + m2)
	for i := 0; i < 3; i++ {
		chars[i], chars[len(chars)-1-i] = chars[len(chars)-1-i], chars[i]
	}
	s := string(chars)
	out := []byte(s + md5Hex(s))
	left, right := 0, len(out)-1
	for i := 0; i < 60; i++ {
		out[left], out[right] = out[right], out[left]
		left++
		right--
	}
	return string(out)
}

// nativeGetSign is the MD5 request signature.
func nativeGetSign(base64ParamString, token string) string {
	return md5Hex(fmt.Sprintf("%s[%s]@%s", magic, md5Hex(token), base64ParamString))
}

// rc4Crypt applies RC4. Decryption is the same operation.
func rc4Crypt(data, key []byte) ([]byte, error) {
	if len(key) == 0 {
		return nil, fmt.Errorf("RC4 密钥为空")
	}
	s := make([]int, 256)
	for i := range s {
		s[i] = i
	}
	j := 0
	for i := 0; i < 256; i++ {
		j = (j + s[i] + int(key[i%len(key)])) & 0xFF
		s[i], s[j] = s[j], s[i]
	}
	out := make([]byte, len(data))
	i, j := 0, 0
	for k, b := range data {
		i = (i + 1) & 0xFF
		j = (j + s[i]) & 0xFF
		s[i], s[j] = s[j], s[i]
		out[k] = b ^ byte(s[(s[i]+s[j])&0xFF])
	}
	return out, nil
}

const rand10Alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

func generateRand10() (string, error) {
	var builder strings.Builder
	builder.Grow(10)
	max := big.NewInt(int64(len(rand10Alphabet)))
	for i := 0; i < 10; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("生成随机数失败: %w", err)
		}
		builder.WriteByte(rand10Alphabet[n.Int64()])
	}
	return builder.String(), nil
}

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func reverseASCII(s string) string {
	out := []byte(s)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return string(out)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstNonZero(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
