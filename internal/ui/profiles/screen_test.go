package profiles

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/SamuelGFDias/file-manager/internal/tool"
	"github.com/SamuelGFDias/file-manager/internal/ui"
)

// fakeProfileSupport é uma implementação trivial de tool.ProfileSupport para
// uso exclusivo nos testes deste pacote.
type fakeProfileSupport struct{}

func (fakeProfileSupport) Empty() any { return &struct{}{} }

func (fakeProfileSupport) Edit(nav *ui.Navigator, current any) (any, error) {
	return current, nil
}

func (fakeProfileSupport) Apply(opts any) (tool.Result, error) {
	return tool.Result{}, nil
}

// fakeTool é uma implementação trivial de tool.Tool para uso exclusivo nos
// testes deste pacote. profile pode ser nil para simular uma ferramenta sem
// suporte a perfis.
type fakeTool struct {
	id      string
	title   string
	profile tool.ProfileSupport
}

func (f fakeTool) Meta() tool.Meta {
	return tool.Meta{ID: f.id, Title: f.title, Description: "ferramenta de teste"}
}

func (f fakeTool) Command() *cobra.Command { return &cobra.Command{} }

func (f fakeTool) Screen() ui.Screen { return nil }

func (f fakeTool) Doc() tool.Doc { return tool.Doc{} }

func (f fakeTool) Profile() tool.ProfileSupport { return f.profile }

func TestSupportingTools_FiltersOnlyToolsWithProfile(t *testing.T) {
	withProfileA := fakeTool{id: "a", title: "Ferramenta A", profile: fakeProfileSupport{}}
	withProfileB := fakeTool{id: "b", title: "Ferramenta B", profile: fakeProfileSupport{}}
	withoutProfile := fakeTool{id: "c", title: "Ferramenta C", profile: nil}

	tools := []tool.Tool{withProfileA, withoutProfile, withProfileB}

	got := SupportingTools(tools)

	if len(got) != 2 {
		t.Fatalf("esperava 2 ferramentas com suporte a perfil, obteve %d: %+v", len(got), got)
	}
	if got[0].Meta().ID != "a" || got[1].Meta().ID != "b" {
		t.Fatalf("esperava ferramentas [a b] na ordem original, obteve [%s %s]", got[0].Meta().ID, got[1].Meta().ID)
	}
}

func TestSupportingTools_EmptyInputReturnsNonNilEmptySlice(t *testing.T) {
	got := SupportingTools([]tool.Tool{})

	if got == nil {
		t.Fatal("esperava slice não-nil para entrada vazia")
	}
	if len(got) != 0 {
		t.Fatalf("esperava slice vazio, obteve %d elementos", len(got))
	}
}

func TestSupportingTools_AllWithoutProfileReturnsEmptySlice(t *testing.T) {
	tools := []tool.Tool{
		fakeTool{id: "a", title: "Ferramenta A", profile: nil},
		fakeTool{id: "b", title: "Ferramenta B", profile: nil},
	}

	got := SupportingTools(tools)

	if got == nil {
		t.Fatal("esperava slice não-nil quando nenhuma ferramenta suporta perfil")
	}
	if len(got) != 0 {
		t.Fatalf("esperava slice vazio, obteve %d elementos", len(got))
	}
}

func TestNewScreen_ReturnsNonNilScreenWithExpectedTitle(t *testing.T) {
	tools := []tool.Tool{
		fakeTool{id: "a", title: "Ferramenta A", profile: fakeProfileSupport{}},
	}

	s := NewScreen(tools)

	if s == nil {
		t.Fatal("esperava tela não-nil")
	}
	if got, want := s.Title(), "Perfis"; got != want {
		t.Fatalf("esperava título %q, obteve %q", want, got)
	}
}
