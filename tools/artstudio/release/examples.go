package main

import (
	"bytes"

	"image/png"
	"path/filepath"

	"aego/art"
)

type lesson struct{ ID, EN, RU, ENText, RUText, ENAction, RUAction, Check string }

var lessons = []lesson{
	{"first-sprite", "Draw your first sprite", "Первый спрайт", "Create a 32×32 drawing. Choose Pencil and draw a short line. Fill a closed shape, then undo and redo.", "Создайте рисунок 32×32. Выберите карандаш, нарисуйте линию и замкнутую фигуру, залейте её. Проверьте отмену и повтор.", "Draw on the canvas; compare with the starting pixels.", "Нарисуйте на холсте; сравните с исходными пикселями.", "pixels"},
	{"layers", "Layers and transparency", "Слои и прозрачность", "Add a raster layer with + in Layers. Paint on it, change opacity, hide it and restore visibility. Do not flatten the source to save it.", "Добавьте растровый слой кнопкой +. Нарисуйте на нём, измените непрозрачность, скройте и верните видимость. Для сохранения исходника сведение не требуется.", "Add at least one layer.", "Добавьте хотя бы один слой.", "layers"},
	{"pixel-perfect", "Pixel perfect and dithering", "Pixel Perfect и дизеринг", "Turn Pixel Perfect on beside the canvas. Draw a stepped curve. Set Dither to 8, draw again and compare. The stroke remains a single Undo operation.", "Включите Pixel Perfect у холста и проведите ступенчатую линию. Установите дизеринг 8 и сравните результат. Штрих остаётся одной операцией отмены.", "Change the drawing with the selected tool.", "Измените рисунок выбранным инструментом.", "pixels"},
	{"indexed", "Palette and Indexed Color", "Палитра и индексированные цвета", "This document uses indexed pixels. Select a palette entry to draw. Palette Order preserves appearance; explicit Remap intentionally changes the image. Save the editable source to keep indices.", "В этом документе пиксели хранят индексы палитры. Выберите цвет и рисуйте. «Порядок» сохраняет внешний вид; явный Remap перекрашивает изображение. Сохраните исходник, чтобы сохранить индексы.", "Draw with a palette index.", "Нарисуйте цветом из палитры.", "pixels"},
	{"animation", "Frame-by-frame animation", "Покадровая анимация", "Use + Frame or Duplicate, then edit a frame and set its duration in milliseconds. Enter starts playback. A sheet export contains PNG plus frame metadata; it is not the native .aeanim export.", "Используйте «+ Кадр» или «Копия», измените рисунок и задайте длительность в миллисекундах. Enter запускает просмотр. Веб-экспорт спрайт-листа содержит PNG и метаданные, а не готовый .aeanim.", "Add a frame to the starting animation.", "Добавьте кадр к исходной анимации.", "frames"},
	{"linked-cels", "Linked cels", "Связанные ячейки", "Select a drawn frame and click Link. Paint on the linked image: both cels change. Unlink before an independent edit. Duplicate makes an independent copy.", "Выберите кадр с рисунком и нажмите «Связать». Изменение общего изображения видно в обеих ячейках. Для независимой правки используйте «Отвязать». «Копия» создаёт независимое изображение.", "Create another cel linked to an existing image.", "Создайте ячейку, связанную с существующим изображением.", "linked"},
	{"onion-skin", "Onion skin", "Калька", "Enable Onion Skin near playback and select an intermediate frame. Paint between the previous and next outlines. Onion skin is a presentation overlay and is not exported into your pixels.", "Включите «Калька» рядом с воспроизведением и выберите промежуточный кадр. Рисуйте между контурами соседних кадров. Калька не включается в экспортируемые пиксели.", "Turn on Onion Skin.", "Включите кальку.", "onion"},
	{"character", "Character animation", "Анимация персонажа", "Edit an eight-frame character loop. Keep the feet aligned and change durations. This web app previews frames; it does not run the project's Animator or change a collider.", "Отредактируйте восьмикадровый цикл. Следите за положением ног и меняйте длительности. Веб-приложение показывает кадры, но не запускает Animator проекта и не меняет коллайдер.", "Edit a frame and retain at least eight frames.", "Измените рисунок, сохранив минимум восемь кадров.", "character"},
	{"surface-channels", "Emission and normal channels", "Emission и normal-каналы", "Choose Emission or Normal above the canvas and paint channel pixels. In Preview choose Lit / Normal, move the Light slider and enable Bloom. Normal vectors are authored as RGB data; they are not inferred from albedo. Height is displayed as a data channel; parallax is not implemented.", "Выберите Emission или Normal у холста и нарисуйте данные канала. В «Предпросмотре» выберите Lit / Normal, переместите свет и включите Bloom. RGB нормали задаются явно, а не вычисляются из albedo. Height показывается как канал данных; parallax не реализован.", "Create or modify an emission/normal cel.", "Создайте или измените ячейку emission/normal.", "channels"},
	{"explosion", "Explosion flipbook", "Покадровый взрыв", "Draw the expansion and fading of an explosion. Preview the eight frames and export a sheet. Choose ParticleSystem2D in Preview or the VFX workspace. Change the emission rate and enable Bloom. This runs the same CPU particle backend as Aego; assignment to a project object is available in native Aego.", "Нарисуйте расширение и затухание взрыва, просмотрите восемь кадров и экспортируйте спрайт-лист. Выберите ParticleSystem2D в предпросмотре или рабочее пространство VFX. Меняйте число частиц в секунду и включайте Bloom. Работает тот же CPU backend Aego; назначение объекту проекта выполняется в native Aego.", "Edit the flipbook while retaining eight frames.", "Измените flipbook, сохранив восемь кадров.", "character"},
	{"aseprite", "Import Aseprite", "Импорт Aseprite", "Use Open to select your .ase/.aseprite. Supported raster, linked cel, layer and tag data is imported by Go; unsupported chunks produce diagnostics. There is no lossless Aseprite round-trip. Download the original point4frames.aseprite fixture linked below, open it locally, and inspect all four frames; its license and provenance are included.", "Откройте собственный .ase/.aseprite. Go-импортёр читает поддерживаемые растровые данные, связанные ячейки, слои и метки; неподдерживаемые chunks вызывают диагностику. Lossless round-trip не заявляется. Скачайте оригинальный point4frames.aseprite по ссылке ниже, откройте его и проверьте четыре кадра; лицензия и происхождение включены.", "Import your Aseprite file; visually inspect diagnostics and frames.", "Импортируйте собственный файл Aseprite и проверьте диагностику и кадры.", "manual"},
	{"export", "Export and use in Aego", "Экспорт и использование в Aego", "Save .aeart, export PNG and a sprite sheet. Reopen the .aeart to continue editing. Native Aego must have the Stage 12.5 importer installed; game assignment and .aeanim are not performed by this standalone app.", "Сохраните .aeart, экспортируйте PNG и спрайт-лист. Снова откройте .aeart для редактирования. В native Aego требуется импортёр Stage 12.5; standalone-приложение не назначает игровой объект и не создаёт .aeanim.", "Download the editable source and reopen it.", "Скачайте редактируемый исходник и откройте его заново.", "manual"},
}

// Checked-in fixtures are decoded with the real Core and rendered here. Keeping
// their stable document IDs makes repeated builds from identical inputs byte-identical.
func writeExamples(out, fixtures string) ([]example, error) {
	var result []example
	for _, l := range lessons {
		b, e := readBounded(filepath.Join(fixtures, l.ID+".aeart"), maxFileBytes)
		if e != nil {
			return nil, e
		}
		d, e := art.Decode(b, art.Limits{})
		if e != nil {
			return nil, e
		}
		im, e := d.Render(d.Info().Frames[0].ID, art.RenderOptions{})
		d.Close()
		if e != nil {
			return nil, e
		}
		var thumbnail bytes.Buffer
		if e = png.Encode(&thumbnail, im); e != nil {
			return nil, e
		}
		file := "examples/" + l.ID + "/start.aeart"
		thumb := "examples/" + l.ID + "/preview.png"
		if e = writeNew(filepath.Join(out, filepath.FromSlash(file)), b); e != nil {
			return nil, e
		}
		if e = writeNew(filepath.Join(out, filepath.FromSlash(thumb)), thumbnail.Bytes()); e != nil {
			return nil, e
		}
		result = append(result, example{l.ID, 1, map[string]string{"en": l.EN, "ru": l.RU}, map[string]string{"en": l.ENText, "ru": l.RUText}, file, thumb, digest(b), len(b)})
	}
	return result, nil
}
