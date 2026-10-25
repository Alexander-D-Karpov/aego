# Установка Aego

Aego — одна программа: редактор, командная строка и среда выполнения. Установите её один раз и создавайте проекты где угодно.

## Загрузка

Кнопки выше ведут к файлам [последнего релиза](https://git.akarpov.ru/sanspie/aego/releases/latest); у каждого файла есть сумма SHA-256 в `SHA256SUMS.txt`. [Ночная сборка](https://git.akarpov.ru/sanspie/aego/releases/tag/nightly) пересобирается после каждого успешного коммита.

- **Windows** — запустите установщик. Он ставит Aego только для вашего пользователя, добавляет `aego` в PATH, кладёт Aego в меню «Пуск» и открывает проекты `.aego` двойным щелчком.
- **macOS** — откройте образ диска и перетащите Aego в «Программы». Приложение пока не нотаризовано: при первом запуске щёлкните правой кнопкой → «Открыть» или разрешите запуск в «Системные настройки» → «Конфиденциальность и безопасность». Релиз может выйти без образа диска: его собирают на Mac отдельно.
- **Linux** — скачайте AppImage, сделайте файл исполняемым и запустите. Добавить его в меню помогут Gear Lever или AppImageLauncher; скрипт установки ниже делает это сам. Есть и архив tar.gz.

## Командная строка

Для серверов, CI и всех, кому удобнее терминал:

```sh
curl -fsSL https://aego.akarpov.ru/install.sh | sh
curl -fsSL https://aego.akarpov.ru/install.sh | sh -s -- --nightly
curl -fsSL https://aego.akarpov.ru/install.sh | sh -s -- --version v0.10.0
```

```powershell
irm https://aego.akarpov.ru/install.ps1 | iex
```

В Windows, чтобы выбрать другую сборку, перед командой задайте `$env:AEGO_CHANNEL = "nightly"` или тег, например `"v0.10.0"`. Скрипт проверяет контрольную сумму, ставит Aego в `~/.local` (или в `%LOCALAPPDATA%\Programs\Aego`) и пишет, что сделал.

## Из исходников

Нужны Go 1.27 и компилятор C для SDL3, а также CMake и Ninja. Первая сборка скачивает и собирает SDL 3 в `.cache/native`.

В Debian и Ubuntu:

```sh
sudo apt-get install -y build-essential cmake ninja-build pkg-config \
  libgl1-mesa-dev libegl1-mesa-dev libgles2-mesa-dev \
  libx11-dev libxext-dev libxrandr-dev libxcursor-dev libxfixes-dev libxi-dev libxss-dev libxtst-dev libxkbcommon-dev \
  libwayland-dev wayland-protocols libdecor-0-dev \
  libasound2-dev libpulse-dev libdrm-dev libgbm-dev libudev-dev libdbus-1-dev libibus-1.0-dev
```

В macOS — `xcode-select --install`, затем `brew install go cmake ninja`. В Windows — оболочка UCRT64 из [MSYS2](https://www.msys2.org/): `pacman -S mingw-w64-ucrt-x86_64-gcc mingw-w64-ucrt-x86_64-cmake mingw-w64-ucrt-x86_64-ninja`.

```sh
git clone https://git.akarpov.ru/sanspie/aego.git
cd aego
make bootstrap && make cli
```

`make cli` собирает редактор и командную строку в `.cache/bin/aego`; `make ci` запускает то же, что CI.

## Требования

OpenGL 3.3 или WebGL2. Подойдёт любой компьютер последних десяти лет; виртуальные машины и удалённые рабочие столы часто не подходят. Если Aego останавливается с сообщением об OpenGL, обновите драйвер видеокарты или запустите его на настоящем железе.

## Нативные расширения на Go

Необязательны. Проектам с собственным кодом на Go нужен установленный Go; редактор сообщит, если не найдёт его. Проекты только со скриптами работают без Go.

Дальше: [Первый проект →](ref:quickstart)
