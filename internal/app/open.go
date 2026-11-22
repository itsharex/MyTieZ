package app

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"mytiez/internal/store"
	"mytiez/locales"

	"github.com/egoist/mygo"
	"golang.org/x/net/html"
)

var (
	shellOpenPath         = mygo.Shell.OpenPath
	shellShowItemInFolder = mygo.Shell.ShowItemInFolder
)

func (a *clipApp) setStatus(msg string) {
	a.pendingToast = msg
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
	item = a.hydrateItem(item)
	appPath := a.openAppForKind(item.Kind)

	switch item.Kind {
	case store.KindText, store.KindURL, store.KindCode:
		trimmed := strings.TrimSpace(item.Text)
		if isHTTPURL(trimmed) || isBareWWWURL(trimmed) {
			targetURL := trimmed
			if strings.HasPrefix(strings.ToLower(targetURL), "www.") {
				targetURL = "https://" + targetURL
			}
			if err := launchURLTarget(appPath, targetURL); err != nil {
				a.setStatus(locales.StatusOpenUrlErr(err.Error()))
				return
			}
			a.hidePanel()
			return
		}
		tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("MyTieZ_entry_%d.txt", item.ID))
		if err := os.WriteFile(tmpPath, []byte(item.Text), 0o600); err != nil {
			a.setStatus(locales.StatusWriteTmpErr(err.Error()))
			return
		}
		a.trackTempFile(tmpPath)
		if err := launchTarget(appPath, tmpPath); err != nil {
			a.setStatus(locales.StatusOpenErr(err.Error()))
			return
		}
		a.hidePanel()

	case store.KindRichText:
		content := item.HTML
		if strings.TrimSpace(content) == "" {
			content = item.Text
		} else {
			content = sanitizeHTMLForExternalOpen(content)
		}
		tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("MyTieZ_entry_%d.html", item.ID))
		if err := os.WriteFile(tmpPath, []byte(content), 0o600); err != nil {
			a.setStatus(locales.StatusWriteTmpErr(err.Error()))
			return
		}
		a.trackTempFile(tmpPath)
		if err := launchTarget(appPath, tmpPath); err != nil {
			a.setStatus(locales.StatusOpenErr(err.Error()))
			return
		}
		a.hidePanel()

	case store.KindImage:
		if item.ImagePath == "" {
			a.setStatus(locales.StatusImgMissing())
			return
		}
		if _, err := os.Stat(item.ImagePath); err != nil {
			a.setStatus(locales.StatusImgMissing())
			return
		}
		if err := launchTarget(appPath, item.ImagePath); err != nil {
			a.setStatus(locales.StatusOpenImgErr(err.Error()))
			return
		}
		a.hidePanel()

	case store.KindFile:
		files := nonEmptyPaths(item.Files)
		switch len(files) {
		case 0:
			a.setStatus(locales.StatusFileMissing())
		case 1:
			a.openFilePath(files[0])
		default:
			a.showFilesInFolder(files)
		}
	}
}

func nonEmptyPaths(files []string) []string {
	out := make([]string, 0, len(files))
	for _, p := range files {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func firstExistingFile(files []string) string {
	for _, p := range files {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func (a *clipApp) openFilePath(p string) {
	if p == "" {
		a.setStatus(locales.StatusFileMissing())
		return
	}
	if _, err := os.Stat(p); err != nil {
		a.setStatus(locales.StatusFileMissing())
		return
	}
	if err := shellOpenPath(p); err != nil {
		a.setStatus(locales.StatusOpenFileErr(err.Error()))
		return
	}
	a.hidePanel()
}

func (a *clipApp) showFileInFolder(p string) {
	if p == "" {
		a.setStatus(locales.StatusFileMissing())
		return
	}
	if _, err := os.Stat(p); err != nil {
		a.setStatus(locales.StatusFileMissing())
		return
	}
	shellShowItemInFolder(p)
	a.hidePanel()
}

func (a *clipApp) showFilesInFolder(files []string) {
	p := firstExistingFile(files)
	if p == "" {
		a.setStatus(locales.StatusFileMissing())
		return
	}
	shellShowItemInFolder(p)
	a.hidePanel()
}

func isBareWWWURL(raw string) bool {
	if raw == "" || strings.ContainsAny(raw, "\r\n \t") {
		return false
	}
	return strings.HasPrefix(strings.ToLower(raw), "www.")
}

func (a *clipApp) trackTempFile(p string) {
	if p == "" {
		return
	}
	if a.tempFiles == nil {
		a.tempFiles = make(map[string]struct{})
	}
	a.tempFiles[p] = struct{}{}
}

func (a *clipApp) removeTempFilesForID(id int64) {
	if id == 0 || len(a.tempFiles) == 0 {
		return
	}
	for _, ext := range []string{".txt", ".html"} {
		p := filepath.Join(os.TempDir(), fmt.Sprintf("MyTieZ_entry_%d%s", id, ext))
		if _, ok := a.tempFiles[p]; ok {
			_ = os.Remove(p)
			delete(a.tempFiles, p)
		}
	}
}

func (a *clipApp) cleanupTempFiles() {
	for p := range a.tempFiles {
		_ = os.Remove(p)
	}
	a.tempFiles = nil
}

func sanitizeHTMLForExternalOpen(raw string) string {
	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return html.EscapeString(raw)
	}
	stripUnsafeHTMLNodes(doc)
	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return html.EscapeString(raw)
	}
	return buf.String()
}

func stripUnsafeHTMLNodes(n *html.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		if c.Type == html.ElementNode {
			tag := strings.ToLower(c.Data)
			switch tag {
			case "script", "iframe", "frame", "frameset", "object", "embed", "applet", "base":
				n.RemoveChild(c)
				c = next
				continue
			case "meta":
				if hasMetaRefreshOrCSP(c.Attr) {
					n.RemoveChild(c)
					c = next
					continue
				}
			}
			c.Attr = filterSafeHTMLAttrs(c.Attr)
		}
		stripUnsafeHTMLNodes(c)
		c = next
	}
}

func hasMetaRefreshOrCSP(attrs []html.Attribute) bool {
	for _, a := range attrs {
		if strings.EqualFold(a.Key, "http-equiv") {
			return true
		}
	}
	return false
}

func filterSafeHTMLAttrs(attrs []html.Attribute) []html.Attribute {
	out := attrs[:0]
	for _, a := range attrs {
		k := strings.ToLower(strings.TrimSpace(a.Key))
		if strings.HasPrefix(k, "on") {
			continue
		}
		if k == "href" || k == "src" || k == "action" || k == "formaction" || k == "xlink:href" {
			v := strings.ToLower(strings.TrimSpace(a.Val))
			if strings.HasPrefix(v, "javascript:") || strings.HasPrefix(v, "vbscript:") || strings.HasPrefix(v, "data:text/html") {
				continue
			}
		}
		out = append(out, a)
	}
	return out
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
	return shellOpenPath(target)
}

func launchWithApp(appPath, target string) error {
	if _, err := os.Stat(appPath); err != nil {
		return fmt.Errorf("%s", locales.ErrAppNotFound(filepath.Base(appPath)))
	}
	return exec.Command(appPath, target).Start()
}

func openAppDisplayName(msgs locales.Messages, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return msgs.OpenAppDefault()
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
		filters = []mygo.FileFilter{{Name: locales.OpenAppFilterExe(), Extensions: []string{"exe"}}}
	}
	a.pickingApp = true
	defer func() {
		a.pickingApp = false
	}()
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:  a.panel,
		Title:   locales.OpenAppDialogTitle(title),
		Filters: filters,
	})
	if err != nil {
		a.setStatus(locales.OpenAppPickErr(err.Error()))
		return
	}
	if len(paths) == 0 || strings.TrimSpace(paths[0]) == "" {
		return
	}
	*dst = paths[0]
	a.persistSettings()
}
