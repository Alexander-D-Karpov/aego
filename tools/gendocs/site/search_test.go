package site

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchProseExcludesExecutableExamples(t *testing.T) {
	md := "# Glow &amp; bloom\n\nUse **Threshold** and `Intensity` with [a lamp](ref:target).\n\n```aescene\nnode hiddenSceneID {}\n```\n\n    hiddenIndentedDump\n\n![hiddenImage](hidden.png)\n\n<script>hiddenScript</script>\n\n## Проверка свечения\n"
	got := searchProse(md)
	for _, want := range []string{"Glow & bloom", "Threshold", "Intensity", "a lamp", "Проверка свечения"} {
		if !strings.Contains(got, want) {
			t.Errorf("lost %q: %s", want, got)
		}
	}
	for _, bad := range []string{"hidden", "ref:target", "```", "<script>"} {
		if strings.Contains(got, bad) {
			t.Errorf("indexed non-prose %q: %s", bad, got)
		}
	}
}

func TestSearchIndexUsesTranslatedProseAndKindLabels(t *testing.T) {
	exp := fixture()
	exp.Entries = append(exp.Entries, Entry{ID: "test-data", Parent: "module/script", Group: "components", Kind: KindType, ValueType: true, Title: "Test data", Body: "English explanation.",
		Translations: map[string]Translation{"ru": {Title: "Данные", Body: "Русское объяснение."}}})
	for i := range exp.Groups {
		if exp.Groups[i].ID == "components" {
			exp.Groups[i].Nav[0].Children = append(exp.Groups[i].Nav[0].Children, Node{ID: "test-data"})
		}
	}
	dir, _, _ := build(t, exp, Options{})
	for _, lang := range []string{"", "ru"} {
		raw, err := os.ReadFile(filepath.Join(dir, lang, "search.json"))
		if err != nil {
			t.Fatal(err)
		}
		var items []searchItem
		if err := json.Unmarshal(raw, &items); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, it := range items {
			if it.ID != "test-data" {
				continue
			}
			found = true
			wantText, wantLabel := "English explanation.", "value type"
			if lang == "ru" {
				wantText, wantLabel = "Русское объяснение.", "структура данных"
			}
			if it.Text != wantText || it.Label != wantLabel || it.Kind != KindType {
				t.Errorf("%s: %+v", lang, it)
			}
		}
		if !found {
			t.Fatal("value type missing from search")
		}
	}
}

func TestBrowserSearchRanker(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable; browser search assertions run in browser gates")
	}
	cmd := exec.Command(node, "-e", browserSearchTests)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("search ranker: %v\n%s", err, out)
	}
}

const browserSearchTests = `
const {test} = require('node:test');
const assert = require('node:assert/strict');
require('./static/search.js');
const {rank, score} = globalThis.AegoDocsSearch;
const data = [
    {i:'render2d.PostProcess', t:'Постобработка камеры', a:['bloom','блум','свечение'], x:'Настройте Threshold.'},
    {i:'animation/sheets', t:'Листы спрайтов', a:['нарезать спрайты'], x:'Нарезка по сетке.'},
    {i:'cli', t:'Команда aego', a:['экспорт в браузер']},
    {i:'editor/agent', t:'Панель «Агент»', a:['агент не видит сцену']},
    {i:'render2d/art', t:'Art Studio', a:['undo','слои','Aseprite'], x:'История изменений рисунка.'},
    {i:'unrelated', t:'Unbound motion'},
    {i:'SHD009', t:'SHD009', s:'Shader link failure'},
    {i:'other', t:'Another article', x:'SHD009 can be reported by the importer.'},
];
test('task phrases and technical IDs rank their target first', () => {
    for (const [q,id] of [['блум','render2d.PostProcess'],['Bloom','render2d.PostProcess'],['свечение','render2d.PostProcess'],['нарезать спрайты','animation/sheets'],['экспорт в браузер','cli'],['агент не видит сцену','editor/agent'],['undo','render2d/art'],['aseprite','render2d/art'],['SHD009','SHD009']]) {
        assert.equal(rank(data,q)[0]?.i,id,q);
    }
});
test('all phrase words are required; headings and prose are searchable', () => {
    assert.equal(rank(data,'Threshold камеры')[0]?.i,'render2d.PostProcess');
    assert.equal(rank(data,'агент не видит банан').length,0);
    assert.equal(score(data[5],'undo'),0);
    assert.equal(rank(data,'').length,0);
});
test('Cyrillic normalization, stable ties and result limits', () => {
    assert.ok(score({i:'x',t:'Тёмная тема'},'темная тема') > 0);
    assert.equal(rank(data,'SHD009',1).length,1);
    const ties=[{i:'z',t:'item'},{i:'a',t:'item'}];
    assert.deepEqual(rank(ties,'item').map(x=>x.i),['a','z']);
});
`
