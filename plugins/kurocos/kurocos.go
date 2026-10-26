package kurocos

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/KasumiYuku/Aurorix/lib/buttons"
	"github.com/KasumiYuku/Aurorix/lib/constant"
	"github.com/KasumiYuku/Aurorix/lib/context"
	"github.com/KasumiYuku/Aurorix/lib/logx"
	"github.com/KasumiYuku/Aurorix/lib/plugin"
	"github.com/KasumiYuku/Aurorix/lib/requests"
)

var logger = logx.New("kurocos")

const (
	kuroListURL = "https://api.kurobbs.com/forum/list"

	kuroOrigin  = "https://www.kurobbs.com"
	kuroReferer = "https://www.kurobbs.com/"
	kuroUA      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36 Edg/136.0.0.0"
	kuroDevCode = "H0O9l04JUG341k5UpUTMNpnGawC5Qt9p"
	kuroVersion = "2.4.4"
	kuroSource  = "h5"
)

const (
	forumCos    = 17 // 鸣潮 COS
	forumDoujin = 11 // 鸣潮同人
)

var (
	client = requests.Init(15)

	cfgMu sync.RWMutex
	cur   = config{
		ParseVideo:      true,
		VideoDefinition: "SD",
		MaxVideoBytes:   10 * 1024 * 1024,
		PageMin:         1,
		PageMax:         200,
		Retries:         3,
	}

	// 每个板块一个独立的顺序计数器, 走到上界绕回下界。
	pageCursor = struct {
		sync.Mutex
		m map[int]int
	}{m: make(map[int]int)}
)

// config 插件运行配置, 由管理面板驱动。
type config struct {
	Token           string // 库街区登录 token, 可选
	ParseVideo      bool   // 是否解析视频帖
	VideoDefinition string // 目标清晰度 FD/LD/SD/HD/OD
	MaxVideoBytes   int64  // 视频大小上限 (字节), 超过直接跳过本帖
	PageMin         int    // 页码范围下界
	PageMax         int    // 页码范围上界
	Retries         int    // 抓取失败/空帖时的重试次数
}

func init() {
	plugin.Register(&plugin.Plugin{
		Id:             "kurocos",
		Name:           "鸣潮COS",
		Description:    "抓取库街区鸣潮 COS 与同人图文/视频",
		Config:         configFields(),
		ValidateConfig: validateConfig,
		ApplyConfig:    applyConfig,
		Commands: []*plugin.Command{
			{
				Prefix:   "鸣潮cos",
				Aliases:  []string{"kurocos"},
				Role:     constant.RoleMember,
				Describe: "随机一张鸣潮 COS 图",
				Handle:   handleCos,
			},
			{
				Prefix:   "鸣潮同人",
				Aliases:  []string{"kurodoujin"},
				Role:     constant.RoleMember,
				Describe: "随机一张鸣潮同人图",
				Handle:   handleDoujin,
			},
		},
	})
}

func configFields() []plugin.ConfigField {
	return []plugin.ConfigField{
		{
			Key:         "token",
			Label:       "库街区 Token",
			Type:        "string",
			Placeholder: "留空使用匿名访问",
			Description: "库街区登录态 token，部分接口需要；留空也能抓大部分公开帖。",
		},
		{
			Key:         "parse_video",
			Label:       "解析视频帖",
			Type:        "boolean",
			Default:     true,
			Description: "开启后遇到视频帖会尝试解析并发送视频（需要本机安装 ffmpeg）。",
		},
		{
			Key:         "video_definition",
			Label:       "视频清晰度",
			Type:        "select",
			Options:     []string{"FD", "LD", "SD", "HD", "OD"},
			Default:     "SD",
			Description: "目标清晰度；没有对应档位时自动退到 m3u8 或第一档。",
		},
		{
			Key:         "max_video_mb",
			Label:       "视频大小上限（MB）",
			Type:        "number",
			Default:     10,
			Description: "超过上限的视频帖直接跳过，抓下一帖。",
		},
		{
			Key:         "page_min",
			Label:       "页码范围下界",
			Type:        "number",
			Default:     1,
			Description: "从第几页开始抓。",
		},
		{
			Key:         "page_max",
			Label:       "页码范围上界",
			Type:        "number",
			Default:     200,
			Description: "抓到这里后绕回下界。",
		},
		{
			Key:         "retries",
			Label:       "重试次数",
			Type:        "number",
			Default:     3,
			Description: "抓到空帖或失败时重试的次数。",
		},
	}
}

func strDefault(m map[string]any, key, def string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return def
}

func boolDefault(m map[string]any, key string, def bool) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return def
}

func intDefault(m map[string]any, key string, def int) int {
	v, ok := m[key]
	if !ok {
		return def
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case string:
		if i, err := strconv.Atoi(n); err == nil {
			return i
		}
	}
	return def
}

func parseConfig(values map[string]any) config {
	cfg := config{
		Token:           strDefault(values, "token", ""),
		ParseVideo:      boolDefault(values, "parse_video", true),
		VideoDefinition: strDefault(values, "video_definition", "SD"),
		MaxVideoBytes:   int64(intDefault(values, "max_video_mb", 10)) * 1024 * 1024,
		PageMin:         intDefault(values, "page_min", 1),
		PageMax:         intDefault(values, "page_max", 200),
		Retries:         intDefault(values, "retries", 3),
	}
	if cfg.PageMin < 1 {
		cfg.PageMin = 1
	}
	if cfg.PageMax < cfg.PageMin {
		cfg.PageMax = cfg.PageMin
	}
	if cfg.Retries < 1 {
		cfg.Retries = 1
	}
	if cfg.MaxVideoBytes <= 0 {
		cfg.MaxVideoBytes = 10 * 1024 * 1024
	}
	return cfg
}

func validateConfig(values map[string]any) error {
	cfg := parseConfig(values)
	switch cfg.VideoDefinition {
	case "FD", "LD", "SD", "HD", "OD":
	default:
		return fmt.Errorf("视频清晰度只支持 FD/LD/SD/HD/OD")
	}
	return nil
}

func applyConfig(values map[string]any) error {
	cfgMu.Lock()
	cur = parseConfig(values)
	cfgMu.Unlock()
	return nil
}

func getConfig() config {
	cfgMu.RLock()
	defer cfgMu.RUnlock()
	return cur
}

func nextPageIndex(forumID int, cfg config) int {
	pageCursor.Lock()
	defer pageCursor.Unlock()
	curPage := pageCursor.m[forumID]
	if curPage < cfg.PageMin {
		curPage = cfg.PageMin - 1
	}
	next := curPage + 1
	if next > cfg.PageMax {
		next = cfg.PageMin
	}
	pageCursor.m[forumID] = next
	return next
}

func kuroHeaders(token string) map[string]string {
	return map[string]string{
		"Accept":       "application/json, text/plain, */*",
		"Content-Type": "application/x-www-form-urlencoded;charset=UTF-8",
		"Origin":       kuroOrigin,
		"Referer":      kuroReferer,
		"User-Agent":   kuroUA,
		"devCode":      kuroDevCode,
		"source":       kuroSource,
		"version":      kuroVersion,
		"token":        token,
	}
}

// kuroListResponse 论坛列表接口回包。
type kuroListResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		PostList []kuroPost `json:"postList"`
	} `json:"data"`
}

// kuroPost 单条帖子。
type kuroPost struct {
	PostType int    `json:"postType"`
	VideoID  string `json:"videoId"`
	UserName string `json:"userName"`
	// ImgContent 图文帖的图片列表。
	ImgContent []imageItem `json:"imgContent"`
	// PostContent 图文帖的正文: 旧接口下是图片数组, 现在通常是正文字符串。
	// 用 RawMessage 兜住两种形态, 解析时再判型。
	PostContent json.RawMessage `json:"postContent"`
}

// imageItem 帖子里的图片。
type imageItem struct {
	URL         string `json:"url"`
	Width       int    `json:"imgWidth"`
	Height      int    `json:"imgHeight"`
	IsAbnormal  bool   `json:"isAbnormal"`
	ContentType *int   `json:"contentType"`
}

// fetched 一次抓取的最终产物: 图片或视频。
type fetched struct {
	kind   string // "image" | "video"
	author string
	img    imageItem
	file   string // 视频本地文件路径
}

func handleCos(ctx *context.MessageContext) error { return handle(ctx, forumCos) }
func handleDoujin(ctx *context.MessageContext) error {
	return handle(ctx, forumDoujin)
}

func handle(ctx *context.MessageContext, forumID int) error {
	cfg := getConfig()
	result := fetchAny(forumID, cfg)
	if result == nil {
		return ctx.Text("未获取到内容").Send()
	}

	if result.kind == "video" {
		defer os.Remove(result.file)
		if err := ctx.Msg().Video(result.file).Send(); err != nil {
			return fmt.Errorf("send kuro video: %w", err)
		}
		md := fmt.Sprintf("转载自库街区：%s", result.author)
		return ctx.Msg().Markdown(md).Keyboard(customKeyboard()).Send()
	}

	md := fmt.Sprintf("%s\n转载自库街区：%s", imageMarkdown(result.img), result.author)
	return ctx.Msg().Markdown(md).Keyboard(customKeyboard()).Send()
}

func imageMarkdown(img imageItem) string {
	if img.Width > 0 && img.Height > 0 {
		return fmt.Sprintf("![图片 #%dpx #%dpx](%s)", img.Width, img.Height, img.URL)
	}
	return fmt.Sprintf("![图片](%s)", img.URL)
}

func fetchAny(forumID int, cfg config) *fetched {
	for i := 0; i < cfg.Retries; i++ {
		result, err := fetchPost(forumID, cfg)
		if err != nil {
			logger.Errorf("第 %d 次抓取失败: %v", i+1, err)
		}
		if result != nil {
			return result
		}
		if i < cfg.Retries-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}
	return nil
}

func fetchPost(forumID int, cfg config) (*fetched, error) {
	page := nextPageIndex(forumID, cfg)
	body := url.Values{}
	body.Set("gameId", "3")
	body.Set("forumId", strconv.Itoa(forumID))
	body.Set("searchType", "3")
	body.Set("pageIndex", strconv.Itoa(page))
	body.Set("pageSize", "1")

	var resp kuroListResponse
	if err := client.PostForm(kuroListURL, body, &resp, kuroHeaders(cfg.Token)); err != nil {
		return nil, err
	}
	if resp.Code != 200 || len(resp.Data.PostList) == 0 {
		return nil, fmt.Errorf("kuro code=%d msg=%s", resp.Code, resp.Msg)
	}

	item := resp.Data.PostList[0]
	author := item.UserName
	if author == "" {
		author = "鸣潮cos"
	}

	// 视频帖: 按配置决定是否解析, 关闭时返回 nil 触发上层重试下一帖。
	if item.PostType == 2 && item.VideoID != "" {
		if !cfg.ParseVideo {
			return nil, nil
		}
		file, err := fetchKuroVideoFile(item.VideoID, cfg)
		if err != nil {
			if isTooLarge(err) {
				logger.Infof("视频超限跳过, 抓下一帖")
				return nil, nil
			}
			return nil, err
		}
		return &fetched{kind: "video", author: author, file: file}, nil
	}

	// 图文帖: 过滤违规图后随机抽一张; 整帖全违规则返回 nil 触发重试。
	imgs := validImages(item)
	if len(imgs) == 0 {
		return nil, nil
	}
	picked := imgs[rand.Intn(len(imgs))]
	return &fetched{kind: "image", author: author, img: picked}, nil
}

// validImages 过滤违规图 (isAbnormal) 与非图片类型 (contentType 缺失或为 2 才保留)。
func validImages(item kuroPost) []imageItem {
	raw := item.ImgContent
	if len(raw) == 0 && len(item.PostContent) > 0 {
		// postContent 可能是图片数组(旧接口), 也可能是正文字符串 —— 只有数组才算图。
		var extra []imageItem
		if json.Unmarshal(item.PostContent, &extra) == nil {
			raw = extra
		}
	}
	out := make([]imageItem, 0, len(raw))
	for _, img := range raw {
		if img.URL == "" || img.IsAbnormal {
			continue
		}
		if img.ContentType != nil && *img.ContentType != 2 {
			continue
		}
		out = append(out, img)
	}
	return out
}

func customKeyboard() *buttons.Keyboard {
	kb := &buttons.Keyboard{}
	b1, _ := kb.AppendButton("cos", "鸣潮cos", "鸣潮cos", buttons.Blue, 0)
	b1.SetAutoCommand("鸣潮cos", true, false).SetUnsupportedTip("鸣潮cos").SetPermission(buttons.AllUser)
	b2, _ := kb.AppendButton("doujin", "鸣潮同人", "鸣潮同人", buttons.Blue, 0)
	b2.SetAutoCommand("鸣潮同人", true, false).SetUnsupportedTip("鸣潮同人").SetPermission(buttons.AllUser)
	b3, _ := kb.AppendButton("menu", "菜单", "菜单", buttons.BlueSolid, 0)
	b3.SetAutoCommand("菜单", true, false).SetUnsupportedTip("菜单").SetPermission(buttons.AllUser)
	return kb
}
