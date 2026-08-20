package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"
)

// Sequencias ANSI. So saem quando o console aceita: em terminal antigo ou com a saida
// redirecionada elas virariam lixo no meio do texto.
const (
	cReset  = "\x1b[0m"
	cBold   = "\x1b[1m"
	cDim    = "\x1b[2m"
	cRed    = "\x1b[31m"
	cGreen  = "\x1b[32m"
	cYellow = "\x1b[33m"
	cCyan   = "\x1b[36m"

	hideCursor = "\x1b[?25l"
	showCursor = "\x1b[?25h"
	wipeLine   = "\r\x1b[2K"
)

type glyphs struct {
	ok, fail, warn, info, step string
	barFull, barEmpty          string
	tl, tr, bl, br, h, v       string
	spinner                    []string
}

var wideGlyphs = glyphs{
	ok: "✔", fail: "✘", warn: "▲", info: "•", step: "▸",
	barFull: "█", barEmpty: "░",
	tl: "╭", tr: "╮", bl: "╰", br: "╯", h: "─", v: "│",
	spinner: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
}

var plainGlyphs = glyphs{
	ok: "+", fail: "x", warn: "!", info: "-", step: ">",
	barFull: "#", barEmpty: ".",
	tl: "+", tr: "+", bl: "+", br: "+", h: "-", v: "|",
	spinner: []string{"|", "/", "-", "\\"},
}

const barWidth = 24

type UI struct {
	mu      sync.Mutex
	out     *os.File
	rich    bool // aceita cor e reescrita de linha
	g       glyphs
	total   int
	step    int
	live    string
	frame   int
	spinner chan struct{}
	started time.Time
}

func NewUI() *UI {
	ansi, wide := prepareConsole(os.Stdout)
	// Convencao do no-color.org: quem exporta a variavel quer texto puro em tudo.
	if os.Getenv("NO_COLOR") != "" {
		ansi, wide = false, false
	}

	u := &UI{out: os.Stdout, rich: ansi, g: plainGlyphs, started: time.Now()}
	if wide {
		u.g = wideGlyphs
	}
	u.trapInterrupt()
	return u
}

// Sem isso um Ctrl+C no meio da busca deixaria o cursor escondido depois que o programa sai.
func (u *UI) trapInterrupt() {
	if !u.rich {
		return
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	go func() {
		<-signals
		u.Close()
		fmt.Fprintln(u.out, "\n  cancelado")
		os.Exit(130)
	}()
}

func (u *UI) Plan(total int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.total = total
}

func (u *UI) paint(code, text string) string {
	if !u.rich || code == "" {
		return text
	}
	return code + text + cReset
}

// TryLock de proposito: o spinner nunca pode segurar quem esta imprimindo de verdade.
func (u *UI) animate() {
	ticker := time.NewTicker(90 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-u.spinner:
			return
		case <-ticker.C:
			if !u.mu.TryLock() {
				continue
			}
			u.frame++
			u.drawLive()
			u.mu.Unlock()
		}
	}
}

func (u *UI) drawLive() {
	if !u.rich || u.live == "" {
		return
	}
	frame := u.g.spinner[u.frame%len(u.g.spinner)]
	fmt.Fprint(u.out, wipeLine+"   "+u.paint(cCyan, frame)+"  "+u.live)
}

func (u *UI) setLive(text string) {
	if !u.rich {
		return
	}
	if u.spinner == nil {
		u.spinner = make(chan struct{})
		fmt.Fprint(u.out, hideCursor)
		go u.animate()
	}
	u.live = text
	u.drawLive()
}

func (u *UI) clearLive() {
	if u.rich && u.live != "" {
		fmt.Fprint(u.out, wipeLine)
	}
	u.live = ""
}

func (u *UI) entry(symbol, code, text string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.clearLive()
	fmt.Fprintf(u.out, "   %s  %s\n", u.paint(code, symbol), text)
}

func (u *UI) Step(format string, args ...any) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.clearLive()
	u.step++

	counter := fmt.Sprintf("%d/%d", u.step, u.total)
	fmt.Fprintf(u.out, "\n %s %s  %s\n",
		u.paint(cCyan, u.g.step),
		u.paint(cDim, counter),
		u.paint(cBold, fmt.Sprintf(format, args...)))
}

func (u *UI) Ok(format string, args ...any) {
	u.entry(u.g.ok, cGreen, fmt.Sprintf(format, args...))
}

func (u *UI) Warn(format string, args ...any) {
	u.entry(u.g.warn, cYellow, fmt.Sprintf(format, args...))
}

func (u *UI) Fail(format string, args ...any) {
	u.entry(u.g.fail, cRed, fmt.Sprintf(format, args...))
}

func (u *UI) Info(format string, args ...any) {
	u.entry(u.g.info, cCyan, fmt.Sprintf(format, args...))
}

func (u *UI) Detail(format string, args ...any) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.clearLive()
	fmt.Fprintf(u.out, "      %s\n", u.paint(cDim, fmt.Sprintf(format, args...)))
}

func (u *UI) Busy(format string, args ...any) {
	text := fmt.Sprintf(format, args...)

	u.mu.Lock()
	defer u.mu.Unlock()
	if !u.rich {
		u.clearLive()
		fmt.Fprintf(u.out, "   %s  %s\n", u.g.info, text)
		return
	}
	u.setLive(u.paint(cDim, text))
}

// Chamada de varias goroutines ao mesmo tempo enquanto as candidatas sao testadas.
func (u *UI) Progress(done, total int, format string, args ...any) {
	if total <= 0 {
		return
	}
	if done > total {
		done = total
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	if !u.rich {
		// Sem reescrita de linha, uma linha por lote ja basta para nao virar spam.
		if done == 0 || (done != total && done%parallelProbes != 0) {
			return
		}
		fmt.Fprintf(u.out, "   %s  %d/%d testadas, %s\n", u.g.info, done, total, fmt.Sprintf(format, args...))
		return
	}

	filled := barWidth * done / total
	bar := strings.Repeat(u.g.barFull, filled) + strings.Repeat(u.g.barEmpty, barWidth-filled)
	u.setLive(fmt.Sprintf("%s  %s  %s",
		u.paint(cCyan, bar),
		fmt.Sprintf("%d/%d", done, total),
		u.paint(cDim, fmt.Sprintf(format, args...))))
}

func runeLen(s string) int {
	return len([]rune(s))
}

// As larguras saem do texto cru: medir depois de colorir contaria os codigos ANSI.
func (u *UI) box(border, title string, rows [][2]string) {
	labels := 0
	for _, row := range rows {
		if n := runeLen(row[0]); n > labels {
			labels = n
		}
	}

	gap := "  "
	if labels == 0 {
		gap = ""
	}

	inner := runeLen(title) + 6
	for _, row := range rows {
		if n := 2 + labels + runeLen(gap) + runeLen(row[1]) + 2; n > inner {
			inner = n
		}
	}

	g := u.g
	head := g.h + g.h
	if title != "" {
		head = g.h + " " + title + " "
	}

	u.mu.Lock()
	defer u.mu.Unlock()
	u.clearLive()

	fmt.Fprintf(u.out, "\n %s\n", u.paint(border, g.tl+head+strings.Repeat(g.h, inner-runeLen(head))+g.tr))
	for _, row := range rows {
		width := 2 + labels + runeLen(gap) + runeLen(row[1])
		cell := "  " + u.paint(cDim, row[0]) + strings.Repeat(" ", labels-runeLen(row[0])) +
			gap + u.paint(cBold, row[1]) + strings.Repeat(" ", inner-width)
		fmt.Fprintf(u.out, " %s%s%s\n", u.paint(border, g.v), cell, u.paint(border, g.v))
	}
	fmt.Fprintf(u.out, " %s\n", u.paint(border, g.bl+strings.Repeat(g.h, inner)+g.br))
}

func (u *UI) Banner() {
	u.box(cCyan, "", [][2]string{
		{"", "DiscordGoLiveBypass"},
		{"", "abre o Discord por um IP de fora, sem VPN no sistema"},
	})
}

func (u *UI) Summary(title string, rows [][2]string) {
	u.box(cGreen, title, append(rows, [2]string{"tempo", u.Elapsed()}))
}

func (u *UI) Elapsed() string {
	elapsed := time.Since(u.started).Round(time.Second)
	if elapsed < time.Minute {
		return fmt.Sprintf("%ds", int(elapsed.Seconds()))
	}
	return fmt.Sprintf("%dmin %ds", int(elapsed.Minutes()), int(elapsed.Seconds())%60)
}

// Quebra por palavra: uma mensagem de erro longa dentro da caixa estouraria a largura.
func wrap(text string, width int) []string {
	var lines []string
	current := ""

	for _, word := range strings.Fields(text) {
		switch {
		case current == "":
			current = word
		case runeLen(current)+1+runeLen(word) <= width:
			current += " " + word
		default:
			lines = append(lines, current)
			current = word
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	if len(lines) == 0 {
		lines = append(lines, "")
	}
	return lines
}

func (u *UI) Fatal(err error) {
	u.Close()

	var rows [][2]string
	for _, line := range wrap(err.Error(), 66) {
		rows = append(rows, [2]string{"", line})
	}
	rows = append(rows,
		[2]string{"", ""},
		[2]string{"", "o FAQ do README tem uma entrada para cada mensagem dessas"})

	u.box(cRed, "nao deu certo", rows)
	fmt.Fprintln(u.out)
}

func (u *UI) Close() {
	u.mu.Lock()
	defer u.mu.Unlock()

	if u.spinner != nil {
		close(u.spinner)
		u.spinner = nil
	}
	u.clearLive()
	if u.rich {
		fmt.Fprint(u.out, showCursor)
	}
}

// O codigo ISO sozinho nao diz nada para quem so quer saber se saiu do Brasil.
var countryNames = map[string]string{
	"AR": "Argentina", "AT": "Austria", "AU": "Australia", "BE": "Belgica",
	"BG": "Bulgaria", "BR": "Brasil", "CA": "Canada", "CH": "Suica",
	"CL": "Chile", "CN": "China", "CO": "Colombia", "CZ": "Tchequia",
	"DE": "Alemanha", "DK": "Dinamarca", "EE": "Estonia", "ES": "Espanha",
	"FI": "Finlandia", "FR": "Franca", "GB": "Reino Unido", "GR": "Grecia",
	"HK": "Hong Kong", "HU": "Hungria", "ID": "Indonesia", "IE": "Irlanda",
	"IL": "Israel", "IN": "India", "IT": "Italia", "JP": "Japao",
	"KR": "Coreia do Sul", "LT": "Lituania", "LU": "Luxemburgo", "LV": "Letonia",
	"MD": "Moldavia", "MX": "Mexico", "MY": "Malasia", "NL": "Holanda",
	"NO": "Noruega", "NZ": "Nova Zelandia", "PE": "Peru", "PH": "Filipinas",
	"PL": "Polonia", "PT": "Portugal", "PY": "Paraguai", "RO": "Romenia",
	"RS": "Servia", "RU": "Russia", "SE": "Suecia", "SG": "Singapura",
	"SK": "Eslovaquia", "TH": "Tailandia", "TR": "Turquia", "TW": "Taiwan",
	"UA": "Ucrania", "US": "Estados Unidos", "UY": "Uruguai", "VN": "Vietna",
	"ZA": "Africa do Sul",
}

func CountryLabel(code string) string {
	if name, ok := countryNames[strings.ToUpper(code)]; ok {
		return fmt.Sprintf("%s (%s)", name, strings.ToUpper(code))
	}
	return strings.ToUpper(code)
}
