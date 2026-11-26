package store

import (
	"database/sql"
	"encoding/json"
	"strings"
)

// Settings holds persisted user preferences for the clipboard manager.
type Settings struct {
	AutoStart                bool     `json:"auto_start"`
	FollowMouse              bool     `json:"follow_mouse"`
	NoFocusMode              bool     `json:"no_focus_mode"`
	AutoHideOnBlur           bool     `json:"auto_hide_on_blur"`
	EdgeDocking              bool     `json:"edge_docking"`
	PinSearchBox             bool     `json:"pin_search_box"`
	ScrollTopButton          bool     `json:"scroll_top_button"`
	Hotkey                   string   `json:"hotkey"`
	OverrideWinV             bool     `json:"override_win_v"`
	Deduplicate              bool     `json:"deduplicate"`
	MaxHistory               int      `json:"max_history"`
	CaptureFiles             bool     `json:"capture_files"`
	CaptureRichText          bool     `json:"capture_rich_text"`
	ClickCopyOnly            bool     `json:"click_copy_only"`
	MoveToTopOnPaste         bool     `json:"move_to_top_on_paste"`
	DeleteAfterPaste         bool     `json:"delete_after_paste"`
	PasteMethod              string   `json:"paste_method"`         // "shift_insert" | "ctrl_v" | "game_mode"
	QuickPasteModifier       string   `json:"quick_paste_modifier"` // "none" | "alt" | "ctrl"
	PlainPasteShortcut       string   `json:"plain_paste_shortcut"` // e.g. "Shift+Enter" (panel-only)
	PrivacyProtection        bool     `json:"privacy_protection"`
	PrivacyKinds             []string `json:"privacy_kinds"`
	PrivacyCustomRules       string   `json:"privacy_custom_rules"`
	SensitiveMaskPrefix      int      `json:"sensitive_mask_prefix"`
	SensitiveMaskSuffix      int      `json:"sensitive_mask_suffix"`
	SensitiveMaskEmailDomain bool     `json:"sensitive_mask_email_domain"`
	ColorMode                string   `json:"color_mode"`    // "system" | "light" | "dark"
	BackdropMode             string   `json:"backdrop_mode"` // "solid" | "mica" | "mica_alt" | "acrylic_base" | "acrylic"
	AccentColor              string   `json:"accent_color"`  // "" (default blue) or "#rrggbb"
	FontFamily               string   `json:"font_family"`   // "" (default Microsoft YaHei UI) or font family name
	CompactMode              bool     `json:"compact_mode"`
	ShowAllCardActions       bool     `json:"show_all_card_actions"`
	HideCardActions          bool     `json:"hide_card_actions"`
	ShowSourceIcon           bool     `json:"show_source_icon"`
	OpenAppText              string   `json:"open_app_text"`
	OpenAppRichText          string   `json:"open_app_rich_text"`
	OpenAppImage             string   `json:"open_app_image"`
	WindowWidth              int      `json:"window_width"`
	WindowHeight             int      `json:"window_height"`
	LastUpdateCheck          int64    `json:"last_update_check"`
}

func DefaultSettings() Settings {
	return Settings{
		AutoStart:           false,
		FollowMouse:         true,
		NoFocusMode:         false,
		AutoHideOnBlur:      true,
		EdgeDocking:         false,
		PinSearchBox:        false,
		ScrollTopButton:     true,
		Hotkey:              "Alt+V",
		Deduplicate:         true,
		MaxHistory:          500,
		CaptureFiles:        true,
		CaptureRichText:     true,
		ClickCopyOnly:       false,
		MoveToTopOnPaste:    true,
		DeleteAfterPaste:    false,
		PasteMethod:         "shift_insert",
		QuickPasteModifier:  "none",
		PlainPasteShortcut:  "Shift+Enter",
		PrivacyProtection:   true,
		PrivacyKinds:        DefaultPrivacyKinds(),
		SensitiveMaskPrefix: 3,
		SensitiveMaskSuffix: 3,
		ColorMode:           "system",
		BackdropMode:        "solid",
		CompactMode:         false,
		ShowAllCardActions:  true,
		HideCardActions:     false,
		ShowSourceIcon:      true,
		WindowWidth:         400,
		WindowHeight:        660,
	}
}

func (s *Store) LoadSettings() (Settings, error) {
	cfg := DefaultSettings()
	var raw string
	err := s.db.QueryRow(`SELECT value FROM app_settings WHERE key = 'config'`).Scan(&raw)
	if err == sql.ErrNoRows {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return DefaultSettings(), err
	}
	if !cfg.NoFocusMode {
		var rawMap map[string]json.RawMessage
		if json.Unmarshal([]byte(raw), &rawMap) == nil {
			if _, ok := rawMap["no_focus_mode"]; !ok {
				var legacy struct {
					HotkeyNoFocus bool `json:"hotkey_no_focus"`
				}
				if json.Unmarshal([]byte(raw), &legacy) == nil && legacy.HotkeyNoFocus {
					cfg.NoFocusMode = true
				}
			}
		}
	}
	if cfg.ColorMode != "system" && cfg.ColorMode != "light" && cfg.ColorMode != "dark" {
		cfg.ColorMode = "system"
	}
	switch cfg.BackdropMode {
	case "solid", "mica", "mica_alt", "acrylic_base", "acrylic":
	default:
		cfg.BackdropMode = DefaultSettings().BackdropMode
	}
	cfg.AccentColor = NormalizeHexColor(cfg.AccentColor)
	cfg.FontFamily = strings.TrimSpace(cfg.FontFamily)
	if cfg.PasteMethod != "shift_insert" && cfg.PasteMethod != "ctrl_v" && cfg.PasteMethod != "game_mode" {
		cfg.PasteMethod = "shift_insert"
	}
	if cfg.QuickPasteModifier != "none" && cfg.QuickPasteModifier != "alt" && cfg.QuickPasteModifier != "ctrl" {
		cfg.QuickPasteModifier = "none"
	}
	if cfg.PrivacyKinds == nil {
		cfg.PrivacyKinds = DefaultPrivacyKinds()
	}
	cfg.SensitiveMaskPrefix = max(0, min(cfg.SensitiveMaskPrefix, 12))
	cfg.SensitiveMaskSuffix = max(0, min(cfg.SensitiveMaskSuffix, 12))
	if cfg.Hotkey == "" {
		cfg.Hotkey = DefaultSettings().Hotkey
	}
	if cfg.MaxHistory < 0 {
		cfg.MaxHistory = 0
	}
	if cfg.WindowWidth < 360 {
		cfg.WindowWidth = DefaultSettings().WindowWidth
	}
	if cfg.WindowHeight < 420 {
		cfg.WindowHeight = DefaultSettings().WindowHeight
	}
	return cfg, nil
}

func (s *Store) SaveSettings(cfg Settings) error {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err = s.db.Exec(`
INSERT INTO app_settings (key, value) VALUES ('config', ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value
`, string(raw))
	return err
}

func (s *Store) ResetSettings() (Settings, error) {
	cfg := DefaultSettings()
	if err := s.SaveSettings(cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// NormalizeHexColor 校验并规范化 #RRGGBB 格式颜色，不合法时返回空字符串。
func NormalizeHexColor(raw string) string {
	s := strings.TrimSpace(raw)
	if len(s) != 7 || s[0] != '#' {
		return ""
	}
	for i := 1; i < 7; i++ {
		c := s[i]
		isHex := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
		if !isHex {
			return ""
		}
	}
	return strings.ToLower(s)
}
