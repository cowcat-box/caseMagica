package app

import (
	"casemagica/internal/styleref"
)

func (a *App) StyleReferences() ([]styleref.Reference, error) {
	return a.interactiveService().StyleReferences()
}

func (s *InteractiveAppService) StyleReferences() ([]styleref.Reference, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DenovaDir == "" {
		return nil, ErrNoWorkspace
	}
	return styleref.NewLibrary(cfg.DenovaDir).List()
}

func (a *App) SaveStyleReference(req styleref.WriteRequest) (styleref.Reference, error) {
	return a.interactiveService().SaveStyleReference(req)
}

func (s *InteractiveAppService) SaveStyleReference(req styleref.WriteRequest) (styleref.Reference, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DenovaDir == "" {
		return styleref.Reference{}, ErrNoWorkspace
	}
	return styleref.NewLibrary(cfg.DenovaDir).Write(req)
}

func (a *App) StyleReferenceFile(path string) (styleref.FileDocument, error) {
	return a.interactiveService().StyleReferenceFile(path)
}

func (s *InteractiveAppService) StyleReferenceFile(path string) (styleref.FileDocument, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DenovaDir == "" {
		return styleref.FileDocument{}, ErrNoWorkspace
	}
	return styleref.NewLibrary(cfg.DenovaDir).Read(path)
}

func (a *App) UpdateStyleReferenceFile(req styleref.UpdateRequest) (styleref.FileDocument, error) {
	return a.interactiveService().UpdateStyleReferenceFile(req)
}

func (s *InteractiveAppService) UpdateStyleReferenceFile(req styleref.UpdateRequest) (styleref.FileDocument, error) {
	cfg := s.cfg()
	if cfg == nil || cfg.DenovaDir == "" {
		return styleref.FileDocument{}, ErrNoWorkspace
	}
	return styleref.NewLibrary(cfg.DenovaDir).Update(req)
}

func (a *App) DeleteStyleReference(path string) error {
	return a.interactiveService().DeleteStyleReference(path)
}

func (s *InteractiveAppService) DeleteStyleReference(path string) error {
	cfg := s.cfg()
	if cfg == nil || cfg.DenovaDir == "" {
		return ErrNoWorkspace
	}
	return styleref.NewLibrary(cfg.DenovaDir).Delete(path)
}
