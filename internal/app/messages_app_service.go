package app

import "casemagica/internal/messages"

func (a *App) Messages(locale string) (messages.ListResult, error) {
	return messages.NewService(a.denovaDir()).ListForLocale(locale)
}

func (a *App) MarkMessageRead(id, locale string) (messages.Message, error) {
	return messages.NewService(a.denovaDir()).MarkReadForLocale(id, locale)
}

func (a *App) MarkAllMessagesRead(locale string) (messages.ListResult, error) {
	return messages.NewService(a.denovaDir()).MarkAllReadForLocale(locale)
}

func (a *App) denovaDir() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.cfg == nil {
		return ""
	}
	return a.cfg.DenovaDir
}
