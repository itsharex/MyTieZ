package app

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"mytiez/internal/store"

	"github.com/egoist/mygo"
)

func (a *clipApp) setStatus(msg string) {
	a.statusMsg = msg
	a.statusUntil = time.Now().Add(2500 * time.Millisecond)
}

func (a *clipApp) openAppForKind(kind store.ClipKind) string {
	switch kind {
	case store.KindText, store.KindURL, store.KindCode:
		return strings.TrimSpace(a.settings.OpenAppText)
	case store.KindRichText:
		return strings.TrimSpace(a.settings.OpenAppRichText)
	case store.KindImage:
		return strings.TrimSpace(a.settings.OpenAppImage)
	default:
		return ""
	}
}

func (a *clipApp) openItem(item store.ClipItem) {
	appPath := a.openAppForKind(item.Kind)

	switch item.Kind {
	case store.KindText, store.KindURL, store.KindCode:
		trimmed := strings.TrimSpace(item.Text)
		if item.Kind == store.KindURL || isHTTPURL(trimmed) {
			targetURL := trimmed
			if strings.HasPrefix(strings.ToLower(targetURL), "www.") {
				targetURL = "https://" + targetURL
			}
			if err := launchURLTarget(appPath, targetURL); err != nil {
				a.setStatus("打开链接失败: " + err.Error())
			}
			return
		}
		tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("MyTieZ_entry_%d.txt", item.ID))
		if err := os.WriteFile(tmpPath, []byte(item.Text), 0o600); err != nil {
			a.setStatus("写入临时文件失败: " + err.Error())
			return
		}
		if err := launchTarget(appPath, tmpPath); err != nil {
			a.setStatus("打开失败: " + err.Error())
		}

	case store.KindRichText:
		content := item.HTML
		if strings.TrimSpace(content) == "" {
			content = item.Text
		}
		tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("MyTieZ_entry_%d.html", item.ID))
		if err := os.WriteFile(tmpPath, []byte(content), 0o600); err != nil {
			a.setStatus("写入临时文件失败: " + err.Error())
			return
		}
		if err := launchTarget(appPath, tmpPath); err != nil {
			a.setStatus("打开失败: " + err.Error())
		}

	case store.KindImage:
		if item.ImagePath == "" {
			a.setStatus("图片文件不存在")
			return
		}
		if _, err := os.Stat(item.ImagePath); err != nil {
			a.setStatus("图片文件不存在")
			return
		}
		if err := launchTarget(appPath, item.ImagePath); err != nil {
			a.setStatus("打开图片失败: " + err.Error())
		}

	case store.KindFile:
		opened := false
		for _, p := range item.Files {
			if p == "" {
				continue
			}
			if _, err := os.Stat(p); err != nil {
				continue
			}
			if err := mygo.Shell.OpenPath(p); err != nil {
				a.setStatus("打开文件失败: " + err.Error())
				return
			}
			opened = true
		}
		if !opened {
			a.setStatus("文件不存在或已移动")
		}
	}
}

func isHTTPURL(raw string) bool {
	if raw == "" || strings.ContainsAny(raw, "\r\n \t") {
		return false
	}
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	return (scheme == "http" || scheme == "https") && u.Host != ""
}

func launchURLTarget(appPath, rawURL string) error {
	if appPath != "" {
		return launchWithApp(appPath, rawURL)
	}
	return mygo.Shell.OpenExternal(rawURL)
}

func launchTarget(appPath, target string) error {
	if appPath != "" {
		return launchWithApp(appPath, target)
	}
	return mygo.Shell.OpenPath(target)
}

func launchWithApp(appPath, target string) error {
	if _, err := os.Stat(appPath); err != nil {
		return fmt.Errorf("未找到指定程序: %s", filepath.Base(appPath))
	}
	return exec.Command(appPath, target).Start()
}

func openAppDisplayName(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "系统默认"
	}
	base := filepath.Base(path)
	for _, ext := range []string{".exe", ".EXE", ".app", ".APP"} {
		if strings.HasSuffix(base, ext) {
			return strings.TrimSuffix(base, ext)
		}
	}
	return base
}

func (a *clipApp) pickOpenApp(title string, dst *string) {
	if dst == nil {
		return
	}
	var filters []mygo.FileFilter
	if runtime.GOOS == "windows" {
		filters = []mygo.FileFilter{{Name: "应用程序", Extensions: []string{"exe"}}}
	}
	a.pickingApp = true
	defer func() {
		a.pickingApp = false
	}()
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:  a.panel,
		Title:   "选择" + title + "默认打开程序",
		Filters: filters,
	})
	if err != nil {
		a.settingsNotice = "选择程序失败: " + err.Error()
		return
	}
	if len(paths) == 0 || strings.TrimSpace(paths[0]) == "" {
		return
	}
	*dst = paths[0]
	a.settingsNotice = "已将" + title + "打开方式设为 " + openAppDisplayName(paths[0])
	a.persistSettings()
}
