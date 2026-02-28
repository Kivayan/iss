package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	mapascii "github.com/Kivayan/map-ascii"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const (
	telemetryInterval = 5 * time.Second
	issURL            = "http://api.open-notify.org/iss-now.json"
	nominatimURL      = "https://nominatim.openstreetmap.org/reverse"
	userAgent         = "iss-tui/1.2 (+https://github.com/kivayan/iss)"
	defaultMapWidth   = 60
	minMapWidth       = 30
	maxMapWidth       = 120
	mapSupersample    = 3
	mapCharAspect     = 2.0
	mapMarginRows     = 1
	markerArmX        = 4
	markerArmY        = 2
)

type telemetryTickMsg time.Time

type telemetryMsg struct {
	country string
	lat     float64
	lon     float64
	err     error
}

type errMsg struct {
	err error
}

type mapFrameMsg struct {
	runID uint64
	frame string
	err   error
}

type mapFrameClosedMsg struct {
	runID uint64
}

type model struct {
	issOver        string
	lat            float64
	lon            float64
	hasCoords      bool
	lastErr        string
	width          int
	height         int
	client         *http.Client
	mapMask        *mapascii.LandMask
	mapASCII       string
	mapFrameCh     chan mapFrameMsg
	cancelMapAnim  context.CancelFunc
	currentAnimRun uint64
}

type issPositionResponse struct {
	Message     string `json:"message"`
	ISSPosition struct {
		Latitude  string `json:"latitude"`
		Longitude string `json:"longitude"`
	} `json:"iss_position"`
}

type nominatimResponse struct {
	Error       string `json:"error"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Category    string `json:"category"`
	Type        string `json:"type"`
	Addresstype string `json:"addresstype"`
	Address     struct {
		Country string `json:"country"`
	} `json:"address"`
}

func main() {
	mask, maskErr := mapascii.LoadEmbeddedDefaultLandMask()
	initialErr := ""
	if maskErr != nil {
		initialErr = fmt.Sprintf("map mask load error: %v", maskErr)
	}

	mapASCII := "Map unavailable."
	if mask != nil {
		rendered, err := renderMap(mask, defaultMapWidth, 0, 0, false)
		if err != nil {
			if initialErr == "" {
				initialErr = fmt.Sprintf("map render error: %v", err)
			}
		} else {
			mapASCII = rendered
		}
	}

	m := model{
		issOver:  "Resolving...",
		mapMask:  mask,
		mapASCII: mapASCII,
		lastErr:  initialErr,
		client: &http.Client{
			Timeout: 8 * time.Second,
		},
	}

	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "application error: %v\n", err)
		os.Exit(1)
	}
}

func (m model) Init() tea.Cmd {
	return telemetryTick(0)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m = m.stopMapAnimation()
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m.syncMapState()

	case telemetryTickMsg:
		return m, tea.Batch(telemetryTick(telemetryInterval), fetchTelemetryCmd(m.client, m.issOver))

	case telemetryMsg:
		m.issOver = msg.country
		m.lat = msg.lat
		m.lon = msg.lon
		m.hasCoords = true
		if msg.err != nil {
			m.lastErr = msg.err.Error()
		} else {
			m.lastErr = ""
		}
		return m.syncMapState()

	case mapFrameMsg:
		if msg.runID != m.currentAnimRun {
			return m, nil
		}
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			return m, waitForMapFrame(m.mapFrameCh, m.currentAnimRun)
		}
		m.mapASCII = msg.frame
		return m, waitForMapFrame(m.mapFrameCh, m.currentAnimRun)

	case mapFrameClosedMsg:
		if msg.runID != m.currentAnimRun {
			return m, nil
		}
		m.mapFrameCh = nil
		m.cancelMapAnim = nil
		return m, nil

	case errMsg:
		m.lastErr = msg.err.Error()
	}

	return m, nil
}

func (m model) View() string {
	telemetryLines := []string{"ISS over: " + m.issOver}
	if m.hasCoords {
		telemetryLines = append(telemetryLines, "Latitude:  "+formatLatitude(m.lat))
		telemetryLines = append(telemetryLines, "Longitude: "+formatLongitude(m.lon))
	} else {
		telemetryLines = append(telemetryLines, "Coords: Resolving...")
	}
	mapView := centerBlock(m.mapASCII, m.width)
	telemetry := centerBlock(telemetryBox(telemetryLines), m.width)
	return "\n" + mapView + "\n\n" + telemetry + "\n"
}

func (m model) syncMapState() (model, tea.Cmd) {
	if m.mapMask == nil {
		return m, nil
	}

	if m.hasCoords {
		return m.startMapAnimation()
	}

	m = m.stopMapAnimation()

	size := mapWidthForTerm(m.width)
	rendered, err := renderMap(m.mapMask, size, m.lat, m.lon, m.hasCoords)
	if err != nil {
		m.lastErr = err.Error()
		return m, nil
	}

	m.mapASCII = rendered
	return m, nil
}

func (m model) cancelMapAnimation() model {
	if m.cancelMapAnim != nil {
		m.cancelMapAnim()
	}
	m.cancelMapAnim = nil
	m.mapFrameCh = nil
	return m
}

func (m model) stopMapAnimation() model {
	m = m.cancelMapAnimation()
	m.currentAnimRun++
	return m
}

func (m model) startMapAnimation() (model, tea.Cmd) {
	size := mapWidthForTerm(m.width)
	marker := &mapascii.Marker{
		Lon:    m.lon,
		Lat:    m.lat,
		Center: 'X',
		ArmX:   markerArmX,
		ArmY:   markerArmY,
	}
	renderOptions := &mapascii.RenderOptions{
		VerticalMarginRows: mapMarginRows,
		Frame:              true,
		ColorMode:          "auto",
		MapColor:           "green",
		MarkerColor:        "blue",
	}
	animOptions := &mapascii.AnimationOptions{
		FPS:   mapascii.DefaultAnimationFPS,
		Style: mapascii.AnimationStyleBlink,
	}

	m = m.cancelMapAnimation()
	m.currentAnimRun++
	runID := m.currentAnimRun

	ctx, cancel := context.WithCancel(context.Background())
	frameCh := make(chan mapFrameMsg, 1)
	m.cancelMapAnim = cancel
	m.mapFrameCh = frameCh

	go streamMapAnimation(ctx, runID, frameCh, m.mapMask, size, marker, renderOptions, animOptions)

	return m, waitForMapFrame(frameCh, runID)
}

func streamMapAnimation(
	ctx context.Context,
	runID uint64,
	frameCh chan<- mapFrameMsg,
	mask *mapascii.LandMask,
	size int,
	marker *mapascii.Marker,
	renderOptions *mapascii.RenderOptions,
	animOptions *mapascii.AnimationOptions,
) {
	defer close(frameCh)

	emit := func(frame mapascii.Frame) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case frameCh <- mapFrameMsg{runID: runID, frame: frame.Text}:
			return nil
		}
	}

	err := mapascii.StreamWorldASCIIAnimation(ctx, mask, size, mapSupersample, mapCharAspect, marker, renderOptions, animOptions, emit)
	if err != nil && !errors.Is(err, context.Canceled) {
		select {
		case <-ctx.Done():
		case frameCh <- mapFrameMsg{runID: runID, err: err}:
		}
	}
}

func waitForMapFrame(frameCh <-chan mapFrameMsg, runID uint64) tea.Cmd {
	if frameCh == nil {
		return nil
	}

	return func() tea.Msg {
		msg, ok := <-frameCh
		if !ok {
			return mapFrameClosedMsg{runID: runID}
		}
		return msg
	}
}

func mapWidthForTerm(termWidth int) int {
	if termWidth <= 0 {
		return defaultMapWidth
	}

	width := termWidth - 4
	if width < minMapWidth {
		return minMapWidth
	}
	if width > maxMapWidth {
		return maxMapWidth
	}

	return width
}

func renderMap(mask *mapascii.LandMask, size int, lat, lon float64, hasCoords bool) (string, error) {
	var marker *mapascii.Marker
	if hasCoords {
		marker = &mapascii.Marker{
			Lon:    lon,
			Lat:    lat,
			Center: 'X',
			ArmX:   markerArmX,
			ArmY:   markerArmY,
		}
	}

	options := &mapascii.RenderOptions{
		VerticalMarginRows: mapMarginRows,
		Frame:              true,
		ColorMode:          "auto",
		MapColor:           "green",
		MarkerColor:        "blue",
	}

	return mapascii.RenderWorldASCIIWithOptions(mask, size, mapSupersample, mapCharAspect, marker, options)
}

func telemetryTick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return telemetryTickMsg(t)
	})
}

func fetchTelemetryCmd(client *http.Client, currentCountry string) tea.Cmd {
	return func() tea.Msg {
		lat, lon, err := fetchISSPosition(client)
		if err != nil {
			return errMsg{err: err}
		}

		country, err := reverseGeocodeCountry(client, lat, lon)
		if err != nil {
			return telemetryMsg{
				country: currentCountry,
				lat:     lat,
				lon:     lon,
				err:     err,
			}
		}

		return telemetryMsg{
			country: country,
			lat:     lat,
			lon:     lon,
		}
	}
}

func fetchISSPosition(client *http.Client) (float64, float64, error) {
	req, err := http.NewRequest(http.MethodGet, issURL, nil)
	if err != nil {
		return 0, 0, err
	}

	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, fmt.Errorf("iss api status: %s", resp.Status)
	}

	var payload issPositionResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, 0, err
	}

	if !strings.EqualFold(payload.Message, "success") {
		return 0, 0, fmt.Errorf("open-notify message: %q", payload.Message)
	}

	lat, err := strconv.ParseFloat(payload.ISSPosition.Latitude, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid latitude %q: %w", payload.ISSPosition.Latitude, err)
	}

	lon, err := strconv.ParseFloat(payload.ISSPosition.Longitude, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid longitude %q: %w", payload.ISSPosition.Longitude, err)
	}

	return lat, lon, nil
}

func reverseGeocodeCountry(client *http.Client, lat, lon float64) (string, error) {
	payload, err := reverseGeocode(client, lat, lon, 3)
	if err != nil {
		return "", err
	}

	if strings.EqualFold(payload.Error, "Unable to geocode") {
		deepPayload, deepErr := reverseGeocode(client, lat, lon, 2)
		if deepErr != nil {
			return "Ocean", nil
		}

		if name := oceanOrWaterName(deepPayload); name != "" {
			return name, nil
		}

		return "Ocean", nil
	}

	if country := strings.TrimSpace(payload.Address.Country); country != "" {
		return country, nil
	}

	if name := oceanOrWaterName(payload); name != "" {
		return name, nil
	}

	deepPayload, err := reverseGeocode(client, lat, lon, 2)
	if err != nil {
		return "Ocean", nil
	}

	if name := oceanOrWaterName(deepPayload); name != "" {
		return name, nil
	}

	return "Ocean", nil
}

func reverseGeocode(client *http.Client, lat, lon float64, zoom int) (nominatimResponse, error) {
	q := url.Values{}
	q.Set("format", "jsonv2")
	q.Set("lat", strconv.FormatFloat(lat, 'f', -1, 64))
	q.Set("lon", strconv.FormatFloat(lon, 'f', -1, 64))
	q.Set("zoom", strconv.Itoa(zoom))
	q.Set("addressdetails", "1")
	q.Set("accept-language", "en")

	u, err := url.Parse(nominatimURL)
	if err != nil {
		return nominatimResponse{}, err
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nominatimResponse{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept-Language", "en")

	resp, err := client.Do(req)
	if err != nil {
		return nominatimResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nominatimResponse{}, fmt.Errorf("nominatim status: %s", resp.Status)
	}

	var payload nominatimResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nominatimResponse{}, err
	}

	return payload, nil
}

func oceanOrWaterName(payload nominatimResponse) string {
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		name = strings.TrimSpace(strings.Split(payload.DisplayName, ",")[0])
	}

	if name == "" {
		return ""
	}

	typeValue := strings.ToLower(strings.TrimSpace(payload.Type))
	category := strings.ToLower(strings.TrimSpace(payload.Category))
	addresstype := strings.ToLower(strings.TrimSpace(payload.Addresstype))
	loweredName := strings.ToLower(name)

	if addresstype == "ocean" || typeValue == "ocean" || typeValue == "sea" || typeValue == "bay" || typeValue == "strait" || category == "natural" {
		return name
	}

	if strings.Contains(loweredName, "ocean") || strings.Contains(loweredName, "sea") || strings.Contains(loweredName, "gulf") || strings.Contains(loweredName, "strait") || strings.Contains(loweredName, "bay") {
		return name
	}

	return ""
}

func telemetryBox(lines []string) string {
	contentWidth := 0
	for _, line := range lines {
		if w := len([]rune(line)); w > contentWidth {
			contentWidth = w
		}
	}

	width := contentWidth + 2
	border := "+" + strings.Repeat("-", width) + "+"

	rendered := make([]string, 0, len(lines)+2)
	rendered = append(rendered, border)
	for _, line := range lines {
		padding := strings.Repeat(" ", contentWidth-len([]rune(line)))
		rendered = append(rendered, "| "+line+padding+" |")
	}
	rendered = append(rendered, border)

	return strings.Join(rendered, "\n")
}

func centerBlock(block string, width int) string {
	if width <= 0 {
		return block
	}

	lines := strings.Split(block, "\n")
	maxWidth := 0
	for _, line := range lines {
		if w := ansi.StringWidth(line); w > maxWidth {
			maxWidth = w
		}
	}

	if maxWidth >= width {
		return block
	}

	leftPad := strings.Repeat(" ", (width-maxWidth)/2)
	for i := range lines {
		lines[i] = leftPad + lines[i]
	}

	return strings.Join(lines, "\n")
}

func formatLatitude(lat float64) string {
	hemisphere := "N"
	value := lat
	if lat < 0 {
		hemisphere = "S"
		value = -lat
	}

	return fmt.Sprintf("%.4f %s", value, hemisphere)
}

func formatLongitude(lon float64) string {
	hemisphere := "E"
	value := lon
	if lon < 0 {
		hemisphere = "W"
		value = -lon
	}

	return fmt.Sprintf("%.4f %s", value, hemisphere)
}
