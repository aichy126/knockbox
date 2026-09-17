package service

import (
	"strconv"
	"sync"
	"time"

	"github.com/aichy126/knockbox/internal/dao"
)

// Settings 运行时可改的设置。
//
// **config.toml 是首次启动的默认值，之后以这里的为准。**
// 两者的分工要说死，否则会出现「我在后台改了，重启又变回去了」或者
// 「我改了配置文件，怎么不生效」——两种困惑都很难自己查出来。
// 规则：某个键在 kv 里存过，就用 kv 的；从没存过，用配置文件的。
type Settings struct {
	d     *dao.DAO
	def   Defaults
	mu    sync.RWMutex
	cache map[string]string
	at    time.Time
}

// Defaults 来自 config.toml。
type Defaults struct {
	PublicEnabled   bool
	RegisterPerHour int
	MaxChannels     int
	MaxPerDay       int
	RetentionDays   int
	SiteName        string
}

const settingsPrefix = "setting."

func NewSettings(d *dao.DAO, def Defaults) *Settings {
	return &Settings{d: d, def: def, cache: map[string]string{}}
}

// load 缓存 5 秒。设置是低频读写，但每个请求都要问一次，
// 不缓存的话相当于给每个请求加一次数据库往返。
func (s *Settings) load() map[string]string {
	s.mu.RLock()
	if time.Since(s.at) < 5*time.Second {
		defer s.mu.RUnlock()
		return s.cache
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.d.Engine().QueryString(
		"SELECT k, v FROM kv WHERE k LIKE ?", settingsPrefix+"%")
	if err == nil {
		m := make(map[string]string, len(rows))
		for _, r := range rows {
			m[r["k"][len(settingsPrefix):]] = r["v"]
		}
		s.cache, s.at = m, time.Now()
	}
	return s.cache
}

func (s *Settings) str(k, def string) string {
	if v, ok := s.load()[k]; ok {
		return v
	}
	return def
}

func (s *Settings) num(k string, def int) int {
	if v, ok := s.load()[k]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func (s *Settings) flag(k string, def bool) bool {
	if v, ok := s.load()[k]; ok {
		return v == "1"
	}
	return def
}

func (s *Settings) PublicEnabled() bool { return s.flag("public_enabled", s.def.PublicEnabled) }
func (s *Settings) RegisterPerHour() int {
	return s.num("register_per_hour", s.def.RegisterPerHour)
}
func (s *Settings) MaxChannels() int   { return s.num("max_channels", s.def.MaxChannels) }
func (s *Settings) MaxPerDay() int     { return s.num("max_per_day", s.def.MaxPerDay) }
func (s *Settings) RetentionDays() int { return s.num("retention_days", s.def.RetentionDays) }
func (s *Settings) SiteName() string   { return s.str("site_name", s.def.SiteName) }

// Limits 给 Quota 用的当前值。
func (s *Settings) Limits() Limits {
	return Limits{
		Enabled:       s.PublicEnabled(),
		MaxChannels:   s.MaxChannels(),
		MaxPerDay:     s.MaxPerDay(),
		RetentionDays: s.RetentionDays(),
	}
}

// Save 写一批设置。空 map 里没提到的键保持不变。
func (s *Settings) Save(vals map[string]string) error {
	now := time.Now().Unix()
	for k, v := range vals {
		if err := s.d.KVSet(settingsPrefix+k, v, now); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.at = time.Time{} // 作废缓存，下次读立刻拿到新值
	s.mu.Unlock()
	return nil
}

// FromConfig 这个键有没有被后台改过。界面上要能看出值的来源，
// 否则用户没法判断改配置文件为什么不生效。
func (s *Settings) FromConfig(k string) bool {
	_, ok := s.load()[k]
	return !ok
}
