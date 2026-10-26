package kurocos

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	kuroVideoCodeURL = "https://api.kurobbs.com/forum/video/refreshPlayCode"
	ffmpegTimeout    = 60 * time.Second
)

var downloadClient = &http.Client{Timeout: 60 * time.Second}

// aliyunSTS 是 refreshPlayCode 里 playAuth(base64) 解码出的阿里云 STS 凭证。
type aliyunSTS struct {
	Region          string `json:"Region"`
	AccessKeyId     string `json:"AccessKeyId"`
	AccessKeySecret string `json:"AccessKeySecret"`
	SecurityToken   string `json:"SecurityToken"`
}

type playCodeResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		PlayAuth string `json:"playAuth"`
	} `json:"data"`
}

type streamInfo struct {
	Definition string `json:"Definition"`
	Format     string `json:"Format"`
	PlayURL    string `json:"PlayURL"`
	Size       string `json:"Size"`
}

type aliyunPlayInfo struct {
	PlayInfoList struct {
		PlayInfo []streamInfo `json:"PlayInfo"`
	} `json:"PlayInfoList"`
	Code    string `json:"Code"`
	Message string `json:"Message"`
}

// tooLargeError 表示视频超过 QQ 上传上限, 上层识别后跳过本帖重试下一帖。
type tooLargeError struct{ actual int64 }

func (e *tooLargeError) Error() string {
	return fmt.Sprintf("video too large: %d bytes", e.actual)
}

func isTooLarge(err error) bool {
	var e *tooLargeError
	return errors.As(err, &e)
}

// fetchKuroVideoFile 按 videoId 拉 playAuth → 阿里云 GetPlayInfo → 挑清晰度 → 落成 mp4 文件。
func fetchKuroVideoFile(videoID string, cfg config) (string, error) {
	sts, err := fetchPlayAuth(videoID, cfg)
	if err != nil {
		return "", err
	}
	info, err := aliyunGetPlayInfo(videoID, sts)
	if err != nil {
		return "", err
	}
	streams := info.PlayInfoList.PlayInfo
	if len(streams) == 0 {
		return "", fmt.Errorf("no playable streams")
	}

	pick := pickStream(streams, cfg.VideoDefinition)
	if pick.PlayURL == "" {
		return "", fmt.Errorf("no PlayURL")
	}

	// 阿里云 PlayInfo 自带 Size, 落盘前先做一次超限判断。
	if declared := parseSize(pick.Size); declared > 0 {
		logger.Infof("命中视频 (%s/%s, %.1fMB)", pick.Definition, pick.Format, float64(declared)/1048576)
		if declared > cfg.MaxVideoBytes {
			return "", &tooLargeError{actual: declared}
		}
	}

	outPath := filepath.Join(os.TempDir(), fmt.Sprintf("kurobbs_%s_%d_%s.mp4", videoID, time.Now().UnixNano(), randHex(3)))
	if pick.Format == "mp4" {
		if err := downloadToFile(pick.PlayURL, outPath); err != nil {
			return "", err
		}
	} else {
		if err := muxHlsToMp4(pick.PlayURL, outPath); err != nil {
			return "", err
		}
	}

	// 兜底: Size 偶尔缺失或 m3u8 实际产物超估算, 落盘后再校一次。
	st, err := os.Stat(outPath)
	if err != nil {
		return "", err
	}
	if st.Size() > cfg.MaxVideoBytes {
		_ = os.Remove(outPath)
		return "", &tooLargeError{actual: st.Size()}
	}
	return outPath, nil
}

func fetchPlayAuth(videoID string, cfg config) (*aliyunSTS, error) {
	body := url.Values{}
	body.Set("videoId", videoID)

	var resp playCodeResponse
	if err := client.PostForm(kuroVideoCodeURL, body, &resp, kuroHeaders(cfg.Token)); err != nil {
		return nil, err
	}
	if resp.Code != 200 {
		return nil, fmt.Errorf("refreshPlayCode code=%d msg=%s", resp.Code, resp.Msg)
	}
	raw, err := base64.StdEncoding.DecodeString(resp.Data.PlayAuth)
	if err != nil {
		return nil, fmt.Errorf("decode playAuth: %w", err)
	}
	var sts aliyunSTS
	if err := json.Unmarshal(raw, &sts); err != nil {
		return nil, fmt.Errorf("parse playAuth: %w", err)
	}
	return &sts, nil
}

// aliyunEncode 阿里云 RPC 规范的 URL 编码。
func aliyunEncode(s string) string {
	return strings.ReplaceAll(
		strings.ReplaceAll(
			strings.ReplaceAll(url.QueryEscape(s), "+", "%20"),
			"*", "%2A"),
		"%7E", "~")
}

// aliyunGetPlayInfo 用 STS 凭证对阿里云 VOD 的 GetPlayInfo 做 HMAC-SHA1 签名并请求。
func aliyunGetPlayInfo(videoID string, sts *aliyunSTS) (*aliyunPlayInfo, error) {
	region := sts.Region
	if region == "" {
		region = "cn-shanghai"
	}
	params := map[string]string{
		"Action":           "GetPlayInfo",
		"Format":           "JSON",
		"Version":          "2017-03-21",
		"AccessKeyId":      sts.AccessKeyId,
		"SecurityToken":    sts.SecurityToken,
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureVersion": "1.0",
		"SignatureNonce":   randHex(16),
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"VideoId":          videoID,
		"Formats":          "mp4,m3u8",
		"AuthTimeout":      "3600",
	}

	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var canonicalParts []string
	for _, k := range keys {
		canonicalParts = append(canonicalParts, aliyunEncode(k)+"="+aliyunEncode(params[k]))
	}
	canonical := strings.Join(canonicalParts, "&")
	stringToSign := "GET&%2F&" + aliyunEncode(canonical)

	mac := hmac.New(sha1.New, []byte(sts.AccessKeySecret+"&"))
	mac.Write([]byte(stringToSign))
	params["Signature"] = base64.StdEncoding.EncodeToString(mac.Sum(nil))

	var qsParts []string
	for k, v := range params {
		qsParts = append(qsParts, aliyunEncode(k)+"="+aliyunEncode(v))
	}
	sort.Strings(qsParts)
	qs := strings.Join(qsParts, "&")

	var info aliyunPlayInfo
	if err := client.Get(fmt.Sprintf("https://vod.%s.aliyuncs.com/?%s", region, qs), &info, nil); err != nil {
		return nil, err
	}
	if info.Code != "" {
		return nil, fmt.Errorf("aliyun vod: %s %s", info.Code, info.Message)
	}
	return &info, nil
}

func pickStream(streams []streamInfo, definition string) streamInfo {
	for _, s := range streams {
		if s.Definition == definition {
			return s
		}
	}
	for _, s := range streams {
		if s.Format == "m3u8" {
			return s
		}
	}
	return streams[0]
}

func parseSize(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// downloadToFile 把直链 mp4 流式写到本地文件。
func downloadToFile(rawURL, outPath string) error {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := downloadClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download %s -> HTTP %d", rawURL, resp.StatusCode)
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// muxHlsToMp4 用 ffmpeg 把 HLS m3u8 remux 成 mp4 (不重新编码)。
func muxHlsToMp4(inputURL, outPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), ffmpegTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-y", "-loglevel", "error",
		"-i", inputURL,
		"-c", "copy",
		"-bsf:a", "aac_adtstoasc",
		outPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("ffmpeg 超时 %s", ffmpegTimeout)
		}
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("ffmpeg 未安装或不在 PATH 中, 请先安装 ffmpeg")
		}
		return fmt.Errorf("ffmpeg exit: %v: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
