package tool

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func TestDocFlags_PreservesOrderAndFields(t *testing.T) {
	params := []Param{
		{
			Name:        "max-depth",
			Shorthand:   "d",
			Type:        "int",
			Description: "profundidade máxima",
			Default:     "1",
			Example:     "3",
		},
		{
			Name:        "verbose",
			Shorthand:   "v",
			Type:        "bool",
			Description: "modo verboso",
			Default:     "false",
			Example:     "true",
		},
		{
			Name:        "output",
			Type:        "string",
			Description: "diretório de saída",
			Default:     "",
			Example:     "./out",
		},
	}

	got := DocFlags(params)

	if len(got) != len(params) {
		t.Fatalf("esperava %d flags, obteve %d", len(params), len(got))
	}

	for i, p := range params {
		want := FlagDoc{
			Name:        p.Name,
			Shorthand:   p.Shorthand,
			Type:        p.Type,
			Default:     p.Default,
			Description: p.Description,
			Example:     p.Example,
		}
		if got[i] != want {
			t.Errorf("posição %d: esperava %+v, obteve %+v", i, want, got[i])
		}
	}
}

func TestDocFlags_NilInputReturnsNonNilEmptySlice(t *testing.T) {
	got := DocFlags(nil)
	if got == nil {
		t.Fatal("esperava slice não-nil para entrada nil")
	}
	if len(got) != 0 {
		t.Fatalf("esperava slice vazio, obteve %d elementos", len(got))
	}
}

func TestDocFlags_EmptyInputReturnsNonNilEmptySlice(t *testing.T) {
	got := DocFlags([]Param{})
	if got == nil {
		t.Fatal("esperava slice não-nil para entrada vazia")
	}
	if len(got) != 0 {
		t.Fatalf("esperava slice vazio, obteve %d elementos", len(got))
	}
}

func TestBindAll_RegistersOnlyParamsWithBindFlag(t *testing.T) {
	fs := pflag.NewFlagSet("teste", pflag.ContinueOnError)

	var maxDepth int
	var noFlagField string

	params := []Param{
		{
			Name: "max-depth",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.IntVar(&maxDepth, "max-depth", 1, "profundidade máxima")
			},
		},
		{
			Name:     "sem-flag",
			BindFlag: nil,
		},
		{
			Name: "shorthand-flag",
			BindFlag: func(fs *pflag.FlagSet) {
				fs.StringVarP(&noFlagField, "shorthand-flag", "s", "", "campo com shorthand")
			},
		},
	}

	BindAll(fs, params)

	if fs.Lookup("max-depth") == nil {
		t.Error("esperava flag \"max-depth\" registrada")
	}
	if fs.Lookup("shorthand-flag") == nil {
		t.Error("esperava flag \"shorthand-flag\" registrada")
	}
	if fs.Lookup("sem-flag") != nil {
		t.Error("não esperava flag \"sem-flag\" registrada, pois BindFlag é nil")
	}
}

func TestPromptAll_ExecutesInOrderAndSkipsNil(t *testing.T) {
	var order []string

	params := []Param{
		{
			Name: "primeiro",
			Prompt: func() error {
				order = append(order, "primeiro")
				return nil
			},
		},
		{
			Name:   "sem-prompt",
			Prompt: nil,
		},
		{
			Name: "segundo",
			Prompt: func() error {
				order = append(order, "segundo")
				return nil
			},
		},
		{
			Name: "terceiro",
			Prompt: func() error {
				order = append(order, "terceiro")
				return nil
			},
		},
	}

	if err := PromptAll(params); err != nil {
		t.Fatalf("não esperava erro, obteve: %v", err)
	}

	want := []string{"primeiro", "segundo", "terceiro"}
	if len(order) != len(want) {
		t.Fatalf("esperava ordem %v, obteve %v", want, order)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("esperava ordem %v, obteve %v", want, order)
		}
	}
}

func TestPromptAll_StopsOnFirstError(t *testing.T) {
	var order []string
	sentinel := errors.New("falha proposital")

	params := []Param{
		{
			Name: "primeiro",
			Prompt: func() error {
				order = append(order, "primeiro")
				return nil
			},
		},
		{
			Name: "segundo-falha",
			Prompt: func() error {
				order = append(order, "segundo-falha")
				return sentinel
			},
		},
		{
			Name: "terceiro",
			Prompt: func() error {
				order = append(order, "terceiro")
				return nil
			},
		},
	}

	err := PromptAll(params)
	if err == nil {
		t.Fatal("esperava erro, obteve nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("esperava erro que envolve o sentinel, obteve: %v", err)
	}
	if !strings.Contains(err.Error(), "segundo-falha") {
		t.Errorf("esperava erro contendo o nome do parâmetro \"segundo-falha\", obteve: %v", err)
	}

	want := []string{"primeiro", "segundo-falha"}
	if len(order) != len(want) {
		t.Fatalf("esperava execução parar após %v, obteve %v", want, order)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("esperava %v, obteve %v", want, order)
		}
	}
}
