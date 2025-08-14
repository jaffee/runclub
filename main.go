package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/felixge/fgprof"
	"github.com/google/uuid"
	"github.com/gorilla/sessions"
)

// User roles
const (
	RoleAdmin   = "admin"
	RoleScanner = "scanner"
	RoleViewer  = "viewer"
)

// Hardcoded users
var users = map[string]struct {
	Password string
	Role     string
}{
	"admin": {
		Password: "admin123",
		Role:     RoleAdmin,
	},
	"scanner": {
		Password: "scanner123",
		Role:     RoleScanner,
	},
	"viewer": {
		Password: "viewer123",
		Role:     RoleViewer,
	},
}

// Season represents a running season
type Season struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	IsActive          bool      `json:"isActive"`
	CreatedAt         time.Time `json:"createdAt"`
	RegistrationToken string    `json:"registrationToken"`
}

// Registration represents a runner registration
type Registration struct {
	ID                  string    `json:"id"`
	SeasonID            *string   `json:"seasonId"`
	FirstName           string    `json:"firstName"`
	LastName            string    `json:"lastName"`
	Grade               string    `json:"grade"`
	Teacher             string    `json:"teacher"`
	Gender              string    `json:"gender"`
	TshirtSize          string    `json:"tshirtSize"`
	ParentFirstName     string    `json:"parentFirstName"`
	ParentLastName      string    `json:"parentLastName"`
	ParentContactNumber string    `json:"parentContactNumber"`
	BackupContactNumber string    `json:"backupContactNumber"`
	ParentEmail         string    `json:"parentEmail"`
	DismissalMethod     string    `json:"dismissalMethod"`
	Allergies           string    `json:"allergies"`
	MedicalInfo         string    `json:"medicalInfo"`
	RegisteredAt        time.Time `json:"registeredAt"`
	Season              *Season   `json:"season,omitempty"`
}

// Track represents a running track/route
type Track struct {
	ID            string    `json:"id"`
	SeasonID      string    `json:"seasonId"`
	Name          string    `json:"name"`
	DistanceMiles float64   `json:"distanceMiles"`
	IsDefault     bool      `json:"isDefault"`
	CreatedAt     time.Time `json:"createdAt"`
}

// ScanRecord represents a record of a QR code scan
type ScanRecord struct {
	ID             string    `json:"id"`
	RegistrationID string    `json:"registrationId"`
	SeasonID       string    `json:"seasonId"`
	TrackID        *string   `json:"trackId,omitempty"`
	ScannedAt      time.Time `json:"scannedAt"`
	RunnerName     string    `json:"runnerName,omitempty"` // Populated for API responses
	Season         *Season   `json:"season,omitempty"`
	Track          *Track    `json:"track,omitempty"`
}

// ScanResult represents the result of a scan operation
type ScanResult struct {
	Success      bool          `json:"success"`
	Message      string        `json:"message"`
	Registration *Registration `json:"registration,omitempty"`
	ScanRecord   *ScanRecord   `json:"scanRecord,omitempty"`
	LapTime      *float64      `json:"lapTime,omitempty"`      // Time in minutes since last scan
	Pace         *float64      `json:"pace,omitempty"`         // Minutes per mile
	PreviousScan *ScanRecord   `json:"previousScan,omitempty"` // Previous scan for reference
}

// PageData holds data to be passed to templates
type PageData struct {
	Title            string
	Registration     *Registration
	ScanResult       *ScanResult
	User             string
	Role             string
	ActiveSeason     *Season
	Seasons          []*Season
	Tracks           []*Track
	SeasonStats      []SeasonStat
	Success          bool
	Message          string
	SuccessCount     int
	ErrorCount       int
	Errors           []string
	Registrations    []*Registration
	SelectedSeasonID string
	SearchQuery      string
	CurrentPage      int
	TotalPages       int
	TotalRunners     int
	RunnersPerPage   int
	BaseURL          string
	Stats            *SeasonStats
	SelectedSeason   *Season
}

// SeasonStat represents statistics for a season
type SeasonStat struct {
	SeasonID    string
	SeasonName  string
	RunnerCount int
	ScanCount   int
}

// RunnerStats represents statistics for a runner
type RunnerStats struct {
	RegistrationID string
	FirstName      string
	LastName       string
	Grade          string
	Teacher        string
	RunCount       int
	TotalDistance  float64
}

// GradeStats represents statistics for a grade
type GradeStats struct {
	Grade         string
	RunnerCount   int
	TotalRuns     int
	TotalDistance float64
	TopRunners    []RunnerStats
}

// SeasonStats represents comprehensive statistics for a season
type SeasonStats struct {
	TotalRunners  int
	TotalRuns     int
	TotalDistance float64
	AveragePerRun float64
	GradeStats    []GradeStats
	TopRunners    []RunnerStats
	TrackUsage    map[string]int
}

var (
	database  *Database
	templates map[string]*template.Template
	store     *sessions.CookieStore
)

func main() {
	log.Println("STARTING UP DOGS!!!!!!!!!!!!")
	// Define command line flags
	port := flag.String("port", "8080", "Port to serve on")
	flag.Parse()

	// Get the current directory
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal("Error getting working directory: ", err)
	}

	// Initialize database
	database, err = NewDatabase()
	if err != nil {
		log.Fatal("Error initializing database: ", err)
	}
	// Close the database when the program exits
	defer database.Close()

	// Initialize session store
	store = sessions.NewCookieStore([]byte("run-club-secret-key"))
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7, // 1 week
		HttpOnly: true,
	}

	// Load templates
	loadTemplates()

	// Create handlers with logging middleware
	http.HandleFunc("/", loggingMiddleware(authMiddleware(homeHandler, []string{RoleAdmin, RoleScanner, RoleViewer})))
	http.HandleFunc("/login", loggingMiddleware(loginHandler))
	http.HandleFunc("/logout", loggingMiddleware(logoutHandler))
	http.HandleFunc("/scan", loggingMiddleware(authMiddleware(scanHandler, []string{RoleAdmin, RoleScanner})))
	http.HandleFunc("/register", loggingMiddleware(authMiddleware(registerHandler, []string{RoleAdmin})))
	http.HandleFunc("/success", loggingMiddleware(authMiddleware(successHandler, []string{RoleAdmin})))
	http.HandleFunc("/seasons", loggingMiddleware(authMiddleware(seasonsHandler, []string{RoleAdmin})))
	http.HandleFunc("/seasons/activate", loggingMiddleware(authMiddleware(activateSeasonHandler, []string{RoleAdmin})))
	http.HandleFunc("/tracks", loggingMiddleware(authMiddleware(tracksHandler, []string{RoleAdmin})))
	http.HandleFunc("/stats", loggingMiddleware(authMiddleware(statsHandler, []string{RoleAdmin, RoleViewer})))
	http.HandleFunc("/csv-upload", loggingMiddleware(authMiddleware(csvUploadHandler, []string{RoleAdmin})))
	http.HandleFunc("/runners", loggingMiddleware(authMiddleware(runnersHandler, []string{RoleAdmin})))
	http.HandleFunc("/runners/export", loggingMiddleware(authMiddleware(runnersExportHandler, []string{RoleAdmin})))
	http.HandleFunc("/badges", loggingMiddleware(authMiddleware(badgesHandler, []string{RoleAdmin})))

	// API endpoints
	http.HandleFunc("/api/registrations", loggingMiddleware(authMiddleware(apiRegistrationsHandler, []string{RoleAdmin})))
	http.HandleFunc("/api/scan", loggingMiddleware(authMiddleware(apiScanHandler, []string{RoleAdmin, RoleScanner})))
	http.HandleFunc("/api/scans", loggingMiddleware(authMiddleware(apiScansHandler, []string{RoleAdmin, RoleScanner})))

	// Public registration endpoints (no auth required)
	http.HandleFunc("/public/register", loggingMiddleware(publicRegisterHandler))
	http.HandleFunc("/public/success", loggingMiddleware(publicSuccessHandler))

	// Debug endpoints (pprof is automatically registered by importing _ "net/http/pprof")
	// This adds: /debug/pprof/, /debug/pprof/cmdline, /debug/pprof/profile, /debug/pprof/symbol, /debug/pprof/trace
	// and /debug/pprof/heap, /debug/pprof/goroutine, /debug/pprof/threadcreate, /debug/pprof/block, /debug/pprof/mutex

	// Add fgprof handler
	http.HandleFunc("/debug/fgprof", func(w http.ResponseWriter, r *http.Request) {
		stop := fgprof.Start(w, fgprof.FormatPprof)
		defer stop()
		time.Sleep(30 * time.Second) // Profile for 30 seconds by default
	})

	// Serve static files (CSS, JS)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(filepath.Join(dir, "static")))))

	// Start the server
	address := fmt.Sprintf(":%s", *port)
	fmt.Printf("Starting Run Club server at http://0.0.0.0%s\n", address)
	fmt.Println("Press Ctrl+C to stop the server")

	// Listen and serve
	err = http.ListenAndServe(address, nil)
	if err != nil {
		log.Fatal("Error starting server: ", err)
	}
}

func loadTemplates() {
	templates = make(map[string]*template.Template)

	// Define template functions
	funcMap := template.FuncMap{
		"add": func(a, b int) int {
			return a + b
		},
		"subtract": func(a, b int) int {
			return a - b
		},
		"multiply": func(a, b int) int {
			return a * b
		},
		"divide": func(a, b int) int {
			if b == 0 {
				return 0
			}
			return a / b
		},
		"sequence": func(start, end int) []int {
			var seq []int
			for i := start; i <= end; i++ {
				seq = append(seq, i)
			}
			return seq
		},
		"slice": func(s string, start, end int) string {
			if start >= len(s) {
				return ""
			}
			if end > len(s) {
				end = len(s)
			}
			return s[start:end]
		},
		"urlquery": func(s string) string {
			return url.QueryEscape(s)
		},
	}

	// Load each template
	templateFiles := []string{"home", "scan", "register", "success", "login", "seasons", "tracks", "csv_upload", "runners", "badges", "stats"}
	for _, name := range templateFiles {
		tmpl, err := template.New(name + ".html").Funcs(funcMap).ParseFiles(fmt.Sprintf("templates/%s.html", name))
		if err != nil {
			log.Fatalf("Error parsing template %s: %v", name, err)
		}
		templates[name] = tmpl
	}
}

// responseWriter wraps http.ResponseWriter to capture response details
type responseWriter struct {
	http.ResponseWriter
	status int
	body   []byte
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	// Capture body for logging (limit to first 1000 bytes for large responses)
	if len(rw.body) < 1000 {
		rw.body = append(rw.body, b...)
	}
	return rw.ResponseWriter.Write(b)
}

// loggingMiddleware logs all requests and responses
func loggingMiddleware(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Log request
		startTime := time.Now()

		// Read and log request body if present
		var requestBody []byte
		if r.Body != nil {
			bodyBytes, err := io.ReadAll(r.Body)
			if err == nil {
				requestBody = bodyBytes
				// Restore the body for the handler
				r.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))
			}
		}

		// Log request details
		log.Printf("REQUEST: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
		if r.URL.RawQuery != "" {
			log.Printf("  Query: %s", r.URL.RawQuery)
		}
		if len(requestBody) > 0 && len(requestBody) < 1000 {
			log.Printf("  Body: %s", string(requestBody))
		} else if len(requestBody) >= 1000 {
			log.Printf("  Body: %s... (truncated)", string(requestBody[:1000]))
		}

		// Wrap response writer to capture response
		wrapped := &responseWriter{
			ResponseWriter: w,
			status:         200, // default status
		}

		// Call the handler
		handler(wrapped, r)

		// Log response
		duration := time.Since(startTime)
		log.Printf("RESPONSE: %s %s - Status: %d - Duration: %v", r.Method, r.URL.Path, wrapped.status, duration)
		if len(wrapped.body) > 0 && len(wrapped.body) < 1000 {
			log.Printf("  Body: %s", string(wrapped.body))
		} else if len(wrapped.body) >= 1000 {
			log.Printf("  Body: %s... (truncated)", string(wrapped.body[:1000]))
		}
	}
}

// authMiddleware checks if the user is authenticated and has the required role
func authMiddleware(handler http.HandlerFunc, roles []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, _ := store.Get(r, "run-club-session")

		// Check if user is authenticated
		if auth, ok := session.Values["authenticated"].(bool); !ok || !auth {
			// Not authenticated, redirect to login
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Check if user has required role
		userRole, ok := session.Values["role"].(string)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Check if the role is allowed
		allowed := false
		for _, role := range roles {
			if userRole == role {
				allowed = true
				break
			}
		}

		if !allowed {
			http.Error(w, "Unauthorized: You don't have permission to access this page", http.StatusForbidden)
			return
		}

		// Call the original handler
		handler(w, r)
	}
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "run-club-session")

	// Already logged in, redirect to home
	if auth, ok := session.Values["authenticated"].(bool); ok && auth {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Handle login form submission
	if r.Method == http.MethodPost {
		r.ParseForm()
		username := r.FormValue("username")
		password := r.FormValue("password")

		// Check credentials
		user, exists := users[username]
		if exists && user.Password == password {
			// Set user as authenticated
			session.Values["authenticated"] = true
			session.Values["username"] = username
			session.Values["role"] = user.Role
			err := session.Save(r, w)
			if err != nil {
				http.Error(w, "Error saving session", http.StatusInternalServerError)
				return
			}

			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		// Render login page with error
		renderTemplate(w, "login", PageData{
			Title: "Run Club - Login",
		})
		return
	}

	// Show login form
	renderTemplate(w, "login", PageData{
		Title: "Run Club - Login",
	})
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "run-club-session")

	// Revoke authentication
	session.Values["authenticated"] = false
	session.Options.MaxAge = -1 // Delete the cookie

	err := session.Save(r, w)
	if err != nil {
		http.Error(w, "Error saving session", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	session, _ := store.Get(r, "run-club-session")
	username := session.Values["username"].(string)
	role := session.Values["role"].(string)

	// Get active season if there is one
	activeSeason, hasActiveSeason, err := database.GetActiveSeason()
	if err != nil {
		log.Printf("Error getting active season: %v", err)
	}

	data := PageData{
		Title: "Run Club - Home",
		User:  username,
		Role:  role,
	}

	if hasActiveSeason {
		data.ActiveSeason = activeSeason
	}

	renderTemplate(w, "home", data)
}

func scanHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "run-club-session")
	username := session.Values["username"].(string)
	role := session.Values["role"].(string)

	// Get active season if there is one
	activeSeason, hasActiveSeason, err := database.GetActiveSeason()
	if err != nil {
		log.Printf("Error getting active season: %v", err)
	}

	data := PageData{
		Title:      "Run Club - Scanner",
		ScanResult: nil,
		User:       username,
		Role:       role,
	}

	if hasActiveSeason {
		data.ActiveSeason = activeSeason

		// Get tracks for the active season
		tracks, err := database.GetTracksBySeasonID(activeSeason.ID)
		if err != nil {
			log.Printf("Error getting tracks: %v", err)
		} else {
			data.Tracks = tracks
		}
	}

	renderTemplate(w, "scan", data)
}

// validatePhoneNumber checks if a phone number matches the format XXX-XXX-XXXX
func validatePhoneNumber(phone string) bool {
	// Check for exact format: 3 digits, dash, 3 digits, dash, 4 digits
	if len(phone) != 12 {
		return false
	}
	
	// Check format XXX-XXX-XXXX
	for i, ch := range phone {
		switch i {
		case 3, 7: // Position of dashes
			if ch != '-' {
				return false
			}
		default: // All other positions should be digits
			if ch < '0' || ch > '9' {
				return false
			}
		}
	}
	
	return true
}

func registerHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "run-club-session")
	username := session.Values["username"].(string)
	role := session.Values["role"].(string)

	// Get active season
	activeSeason, hasActiveSeason, err := database.GetActiveSeason()
	if err != nil {
		log.Printf("Error getting active season: %v", err)
	}

	// For GET requests, just show the form
	if r.Method == http.MethodGet {
		data := PageData{
			Title: "Run Club - Register",
			User:  username,
			Role:  role,
		}

		// Add active season data if available
		if hasActiveSeason {
			data.ActiveSeason = activeSeason
		}

		renderTemplate(w, "register", data)
		return
	}

	// For POST requests, process the form submission
	if r.Method == http.MethodPost {
		// Check if there's an active season
		if !hasActiveSeason {
			http.Error(w, "Cannot register runners without an active season", http.StatusBadRequest)
			return
		}

		// Parse form data
		err := r.ParseForm()
		if err != nil {
			http.Error(w, "Error parsing form", http.StatusBadRequest)
			return
		}

		// Validate phone numbers
		parentPhone := r.FormValue("parentContactNumber")
		if !validatePhoneNumber(parentPhone) {
			http.Error(w, "Invalid parent/guardian contact number. Please use format: 123-456-7890", http.StatusBadRequest)
			return
		}
		
		backupPhone := r.FormValue("backupContactNumber")
		if backupPhone != "" && !validatePhoneNumber(backupPhone) {
			http.Error(w, "Invalid backup contact number. Please use format: 123-456-7890", http.StatusBadRequest)
			return
		}

		// Create a new registration
		reg := &Registration{
			ID:                  uuid.New().String(),
			SeasonID:            &activeSeason.ID,
			FirstName:           r.FormValue("firstName"),
			LastName:            r.FormValue("lastName"),
			Grade:               r.FormValue("grade"),
			Teacher:             r.FormValue("teacher"),
			Gender:              r.FormValue("gender"),
			TshirtSize:          r.FormValue("tshirtSize"),
			ParentFirstName:     r.FormValue("parentFirstName"),
			ParentLastName:      r.FormValue("parentLastName"),
			ParentContactNumber: r.FormValue("parentContactNumber"),
			BackupContactNumber: r.FormValue("backupContactNumber"),
			ParentEmail:         r.FormValue("parentEmail"),
			DismissalMethod:     r.FormValue("dismissalMethod"),
			Allergies:           r.FormValue("allergies"),
			MedicalInfo:         r.FormValue("medicalInfo"),
			RegisteredAt:        time.Now(),
			Season:              activeSeason,
		}

		// Save the registration to the database
		err = database.SaveRegistration(reg)
		if err != nil {
			log.Printf("Error saving registration: %v", err)
			http.Error(w, "Failed to save registration", http.StatusInternalServerError)
			return
		}

		// Redirect to success page
		http.Redirect(w, r, fmt.Sprintf("/success?id=%s", reg.ID), http.StatusSeeOther)
		return
	}

	// Method not allowed for other HTTP methods
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func successHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "run-club-session")
	username := session.Values["username"].(string)
	role := session.Values["role"].(string)

	// Get the registration ID from URL query parameters
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Missing registration ID", http.StatusBadRequest)
		return
	}

	// Look up the registration
	reg, exists, err := database.GetRegistration(id)
	if err != nil {
		log.Printf("Error getting registration: %v", err)
		http.Error(w, "Failed to retrieve registration", http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "Registration not found", http.StatusNotFound)
		return
	}

	// Render the success page with registration details
	renderTemplate(w, "success", PageData{
		Title:        "Registration Successful",
		Registration: reg,
		User:         username,
		Role:         role,
	})
}

func seasonsHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "run-club-session")
	username := session.Values["username"].(string)
	role := session.Values["role"].(string)

	// For GET requests, show the seasons management page
	if r.Method == http.MethodGet {
		// Get all seasons
		seasons, err := database.GetAllSeasons()
		if err != nil {
			log.Printf("Error getting seasons: %v", err)
			http.Error(w, "Failed to retrieve seasons", http.StatusInternalServerError)
			return
		}

		// Get active season
		activeSeason, _, err := database.GetActiveSeason()
		if err != nil {
			log.Printf("Error getting active season: %v", err)
		}

		// Get statistics for each season
		var seasonStats []SeasonStat
		for _, season := range seasons {
			// Get counts for each season
			regCount, _ := database.GetRegistrationCountForSeason(season.ID)
			scanCount, _ := database.GetScanCountForSeason(season.ID)

			seasonStats = append(seasonStats, SeasonStat{
				SeasonID:    season.ID,
				SeasonName:  season.Name,
				RunnerCount: regCount,
				ScanCount:   scanCount,
			})
		}

		// Construct base URL from request
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		baseURL := fmt.Sprintf("%s://%s", scheme, r.Host)

		// Render the seasons page
		renderTemplate(w, "seasons", PageData{
			Title:        "Run Club - Manage Seasons",
			User:         username,
			Role:         role,
			ActiveSeason: activeSeason,
			Seasons:      seasons,
			SeasonStats:  seasonStats,
			BaseURL:      baseURL,
		})
		return
	}

	// For POST requests, create a new season
	if r.Method == http.MethodPost {
		// Parse form data
		err := r.ParseForm()
		if err != nil {
			http.Error(w, "Error parsing form", http.StatusBadRequest)
			return
		}

		// Create a new season
		seasonName := r.FormValue("name")
		if seasonName == "" {
			http.Error(w, "Season name is required", http.StatusBadRequest)
			return
		}

		// Determine if it should be active
		isActive := r.FormValue("is_active") == "true"

		// Create the season
		season := &Season{
			ID:        uuid.New().String(),
			Name:      seasonName,
			IsActive:  isActive,
			CreatedAt: time.Now(),
		}

		// Save the season
		err = database.SaveSeason(season)
		if err != nil {
			log.Printf("Error saving season: %v", err)
			http.Error(w, "Failed to save season", http.StatusInternalServerError)
			return
		}

		// Redirect back to seasons page
		http.Redirect(w, r, "/seasons", http.StatusSeeOther)
		return
	}

	// Method not allowed for other HTTP methods
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func activateSeasonHandler(w http.ResponseWriter, r *http.Request) {
	// Only allow POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse form data
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	// Get the season ID to activate
	seasonID := r.FormValue("id")
	if seasonID == "" {
		http.Error(w, "Season ID is required", http.StatusBadRequest)
		return
	}

	// Activate the season
	err = database.SetActiveSeason(seasonID)
	if err != nil {
		log.Printf("Error activating season: %v", err)
		http.Error(w, "Failed to activate season", http.StatusInternalServerError)
		return
	}

	// Redirect back to seasons page
	http.Redirect(w, r, "/seasons", http.StatusSeeOther)
}

func tracksHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "run-club-session")
	username := session.Values["username"].(string)
	role := session.Values["role"].(string)

	// Get active season
	activeSeason, hasActiveSeason, err := database.GetActiveSeason()
	if err != nil {
		log.Printf("Error getting active season: %v", err)
	}

	// For GET requests, show the tracks management page
	if r.Method == http.MethodGet {
		data := PageData{
			Title: "Run Club - Manage Tracks",
			User:  username,
			Role:  role,
		}

		if hasActiveSeason {
			data.ActiveSeason = activeSeason
			// Get tracks for the active season
			tracks, err := database.GetTracksBySeasonID(activeSeason.ID)
			if err != nil {
				log.Printf("Error getting tracks: %v", err)
			} else {
				data.Tracks = tracks
			}
		}

		renderTemplate(w, "tracks", data)
		return
	}

	// For POST requests, create a new track
	if r.Method == http.MethodPost {
		if !hasActiveSeason {
			http.Error(w, "Cannot create tracks without an active season", http.StatusBadRequest)
			return
		}

		// Parse form data
		err := r.ParseForm()
		if err != nil {
			http.Error(w, "Error parsing form", http.StatusBadRequest)
			return
		}

		// Parse distance
		distanceStr := r.FormValue("distance_miles")
		distance, err := strconv.ParseFloat(distanceStr, 64)
		if err != nil {
			http.Error(w, "Invalid distance value", http.StatusBadRequest)
			return
		}

		// Create the track
		track := &Track{
			ID:            uuid.New().String(),
			SeasonID:      activeSeason.ID,
			Name:          r.FormValue("name"),
			DistanceMiles: distance,
			IsDefault:     r.FormValue("is_default") == "true",
			CreatedAt:     time.Now(),
		}

		// Save the track
		err = database.SaveTrack(track)
		if err != nil {
			log.Printf("Error saving track: %v", err)
			http.Error(w, "Failed to save track", http.StatusInternalServerError)
			return
		}

		// Redirect back to tracks page
		http.Redirect(w, r, "/tracks", http.StatusSeeOther)
		return
	}

	// Method not allowed for other HTTP methods
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func apiRegistrationsHandler(w http.ResponseWriter, r *http.Request) {
	// Get season ID from query parameter, if any
	seasonID := r.URL.Query().Get("season_id")

	// Handle GET for retrieving all registrations
	if r.Method == http.MethodGet {
		// Get all registrations, optionally filtered by season
		regs, err := database.GetAllRegistrations(seasonID)
		if err != nil {
			log.Printf("Error retrieving registrations: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Set response headers
		w.Header().Set("Content-Type", "application/json")

		// Encode and write the response
		err = json.NewEncoder(w).Encode(regs)
		if err != nil {
			log.Printf("Error encoding registrations: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	// Method not allowed for other HTTP methods
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// statsHandler displays comprehensive statistics for a season
func statsHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "run-club-session")
	username := session.Values["username"].(string)
	role := session.Values["role"].(string)

	// Get seasons for selector
	seasons, err := database.GetAllSeasons()
	if err != nil {
		log.Printf("Error getting seasons: %v", err)
		http.Error(w, "Failed to retrieve seasons", http.StatusInternalServerError)
		return
	}

	// Get active season
	activeSeason, hasActiveSeason, err := database.GetActiveSeason()
	if err != nil {
		log.Printf("Error getting active season: %v", err)
	}

	// Get selected season from query parameter or default to active season
	seasonID := r.URL.Query().Get("season_id")
	if seasonID == "" && hasActiveSeason {
		seasonID = activeSeason.ID
	}

	// Get statistics for the selected season
	var stats *SeasonStats
	if seasonID != "" {
		stats, err = database.GetSeasonStatistics(seasonID)
		if err != nil {
			log.Printf("Error getting season statistics: %v", err)
			http.Error(w, "Failed to retrieve statistics", http.StatusInternalServerError)
			return
		}
	}

	// Find selected season
	var selectedSeason *Season
	for _, s := range seasons {
		if s.ID == seasonID {
			selectedSeason = s
			break
		}
	}

	data := PageData{
		Title:            "Run Club - Statistics",
		User:             username,
		Role:             role,
		Seasons:          seasons,
		ActiveSeason:     activeSeason,
		SelectedSeasonID: seasonID,
		Stats:            stats,
		SelectedSeason:   selectedSeason,
	}

	renderTemplate(w, "stats", data)
}

func apiScanHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse JSON request
	var request struct {
		Code    string  `json:"code"`
		TrackID *string `json:"trackId,omitempty"`
	}

	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		log.Printf("Error decoding scan request: %v", err)
		sendJSONResponse(w, ScanResult{
			Success: false,
			Message: "Invalid request format",
		}, http.StatusBadRequest)
		return
	}

	// Check if the code is a valid UUID
	_, err = uuid.Parse(request.Code)
	if err != nil {
		sendJSONResponse(w, ScanResult{
			Success: false,
			Message: "Invalid QR code format",
		}, http.StatusOK)
		return
	}

	// Record the scan
	log.Printf("Recording scan for code: %s", request.Code)
	scan, reg, err := database.RecordScan(request.Code, request.TrackID)
	if err != nil {
		// Check if it's a debounce error
		if strings.Contains(err.Error(), "scan rejected") {
			sendJSONResponse(w, ScanResult{
				Success: false,
				Message: err.Error(),
			}, http.StatusOK)
		} else {
			sendJSONResponse(w, ScanResult{
				Success: false,
				Message: "Runner not found",
			}, http.StatusOK)
		}
		return
	}

	// Get previous scan to calculate lap time and pace
	previousScan, err := database.GetPreviousScan(request.Code, scan.ScannedAt)
	if err != nil {
		log.Printf("Error getting previous scan: %v", err)
		// Continue without lap time data
	}

	result := ScanResult{
		Success:      true,
		Message:      fmt.Sprintf("Successfully recorded run for %s %s", reg.FirstName, reg.LastName),
		Registration: reg,
		ScanRecord:   scan,
		PreviousScan: previousScan,
	}

	// Calculate lap time and pace if we have a previous scan
	if previousScan != nil {
		lapTime := scan.ScannedAt.Sub(previousScan.ScannedAt).Minutes()
		result.LapTime = &lapTime

		// Calculate pace if we have track distance
		if scan.Track != nil && scan.Track.DistanceMiles > 0 {
			pace := lapTime / scan.Track.DistanceMiles
			result.Pace = &pace
		}
	}

	// Return success response
	sendJSONResponse(w, result, http.StatusOK)
}

func apiScansHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept GET requests
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if a registration ID is provided
	regID := r.URL.Query().Get("registration_id")
	seasonID := r.URL.Query().Get("season_id")

	var scans []*ScanRecord
	var err error

	if regID != "" {
		// Get scans for a specific registration
		scans, err = database.GetScansByRegistrationID(regID)
	} else {
		// Get all scans, optionally filtered by season
		scans, err = database.GetAllScans(seasonID)
	}

	if err != nil {
		log.Printf("Error retrieving scans: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/json")

	// Encode and write the response
	err = json.NewEncoder(w).Encode(scans)
	if err != nil {
		log.Printf("Error encoding scans: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func sendJSONResponse(w http.ResponseWriter, data interface{}, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	err := json.NewEncoder(w).Encode(data)
	if err != nil {
		log.Printf("Error encoding JSON response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func csvUploadHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "run-club-session")
	username := session.Values["username"].(string)
	role := session.Values["role"].(string)

	// Get active season
	activeSeason, hasActiveSeason, err := database.GetActiveSeason()
	if err != nil {
		log.Printf("Error getting active season: %v", err)
	}

	// Create base page data
	data := PageData{
		Title: "Run Club - Bulk Register",
		User:  username,
		Role:  role,
	}

	// Add active season data if available
	if hasActiveSeason {
		data.ActiveSeason = activeSeason
	}

	// For GET requests, just show the form
	if r.Method == http.MethodGet {
		renderTemplate(w, "csv_upload", data)
		return
	}

	// For POST requests, process the CSV file
	if r.Method == http.MethodPost {
		// Check if there's an active season
		if !hasActiveSeason {
			data.Success = false
			data.Message = "Cannot register runners without an active season"
			renderTemplate(w, "csv_upload", data)
			return
		}

		// Parse the multipart form to get the file
		err := r.ParseMultipartForm(10 << 20) // Max 10MB
		if err != nil {
			data.Success = false
			data.Message = "Error parsing form"
			renderTemplate(w, "csv_upload", data)
			return
		}

		// Get the file from the form
		file, header, err := r.FormFile("csv-file")
		if err != nil {
			data.Success = false
			data.Message = "Error retrieving file from form"
			renderTemplate(w, "csv_upload", data)
			return
		}
		defer file.Close()

		// Check if the file is a CSV
		if !strings.HasSuffix(header.Filename, ".csv") {
			data.Success = false
			data.Message = "Uploaded file is not a CSV"
			renderTemplate(w, "csv_upload", data)
			return
		}

		// Process the CSV file and register runners
		successCount, errorCount, errors := processCsvFile(file, activeSeason)

		// Prepare data for the template
		data.Success = errorCount == 0
		data.SuccessCount = successCount
		data.ErrorCount = errorCount
		data.Errors = errors

		if data.Success {
			data.Message = "Successfully registered all runners!"
		} else {
			if successCount > 0 {
				data.Message = "Partially successful registration. See errors below."
			} else {
				data.Message = "Failed to register runners. See errors below."
			}
		}

		renderTemplate(w, "csv_upload", data)
		return
	}

	// Method not allowed for other HTTP methods
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// processCsvFile processes a CSV file and registers the runners
func processCsvFile(file io.Reader, season *Season) (successCount, errorCount int, errors []string) {
	// Create a new CSV reader
	reader := csv.NewReader(file)

	// Read header row first to validate
	header, err := reader.Read()
	if err != nil {
		return 0, 1, []string{"Error reading CSV header: " + err.Error()}
	}

	// Check for expected headers
	expectedHeaders := []string{"FirstName", "LastName", "Grade", "Teacher", "Gender", "ParentContactNumber", "BackupContactNumber", "ParentEmail", "DismissalMethod", "Allergies", "MedicalInfo"}
	if !reflect.DeepEqual(header, expectedHeaders) {
		return 0, 1, []string{"CSV headers do not match expected format. Please use the template provided."}
	}

	// Process rows
	lineNum := 1 // Start at line 1 (header row)
	for {
		lineNum++
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			errors = append(errors, fmt.Sprintf("Error reading line %d: %v", lineNum, err))
			errorCount++
			continue
		}

		// Skip empty rows
		if len(row) == 0 || (len(row) == 1 && row[0] == "") {
			continue
		}

		// Validate row length
		if len(row) != len(expectedHeaders) {
			errors = append(errors, fmt.Sprintf("Line %d: Wrong number of columns", lineNum))
			errorCount++
			continue
		}

		// Extract values
		firstName := strings.TrimSpace(row[0])
		lastName := strings.TrimSpace(row[1])
		grade := strings.TrimSpace(row[2])
		teacher := strings.TrimSpace(row[3])
		gender := strings.TrimSpace(row[4])
		parentContactNumber := strings.TrimSpace(row[5])
		backupContactNumber := strings.TrimSpace(row[6])
		parentEmail := strings.TrimSpace(row[7])
		dismissalMethod := strings.TrimSpace(row[8])
		allergies := strings.TrimSpace(row[9])
		medicalInfo := strings.TrimSpace(row[10])

		// Validate required fields
		if firstName == "" || lastName == "" || grade == "" || teacher == "" || gender == "" || parentContactNumber == "" || parentEmail == "" || dismissalMethod == "" {
			errors = append(errors, fmt.Sprintf("Line %d: Missing required field(s)", lineNum))
			errorCount++
			continue
		}

		// Validate grade
		validGrades := map[string]bool{"K": true, "1": true, "2": true, "3": true, "4": true, "5": true}
		if !validGrades[grade] {
			errors = append(errors, fmt.Sprintf("Line %d: Invalid grade '%s'", lineNum, grade))
			errorCount++
			continue
		}

		// Validate gender
		validGenders := map[string]bool{"Male": true, "Female": true, "Other": true, "Prefer not to say": true}
		if !validGenders[gender] {
			errors = append(errors, fmt.Sprintf("Line %d: Invalid gender '%s'", lineNum, gender))
			errorCount++
			continue
		}

		// Validate dismissal method
		validDismissalMethods := map[string]bool{"Walking unescorted": true, "Picked up by adult": true, "Clayton Crew": true}
		if !validDismissalMethods[dismissalMethod] {
			errors = append(errors, fmt.Sprintf("Line %d: Invalid dismissal method '%s'", lineNum, dismissalMethod))
			errorCount++
			continue
		}

		// Create registration
		reg := &Registration{
			ID:                  uuid.New().String(),
			SeasonID:            &season.ID,
			FirstName:           firstName,
			LastName:            lastName,
			Grade:               grade,
			Teacher:             teacher,
			Gender:              gender,
			ParentContactNumber: parentContactNumber,
			BackupContactNumber: backupContactNumber,
			ParentEmail:         parentEmail,
			DismissalMethod:     dismissalMethod,
			Allergies:           allergies,
			MedicalInfo:         medicalInfo,
			RegisteredAt:        time.Now(),
			Season:              season,
		}

		// Save to database
		err = database.SaveRegistration(reg)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Line %d: Error saving to database: %v", lineNum, err))
			errorCount++
			continue
		}

		successCount++
	}

	return successCount, errorCount, errors
}

func runnersHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "run-club-session")
	username := session.Values["username"].(string)
	role := session.Values["role"].(string)

	// Get all seasons
	seasons, err := database.GetAllSeasons()
	if err != nil {
		log.Printf("Error getting seasons: %v", err)
		http.Error(w, "Failed to retrieve seasons", http.StatusInternalServerError)
		return
	}

	// Get active season
	activeSeason, _, err := database.GetActiveSeason()
	if err != nil {
		log.Printf("Error getting active season: %v", err)
	}

	// Parse query parameters
	seasonID := r.URL.Query().Get("season_id")

	// Make sure we HTML-escape the search query for template safety
	searchQuery := template.HTMLEscapeString(r.URL.Query().Get("search"))

	pageStr := r.URL.Query().Get("page")
	page := 1
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}
	perPage := 20 // Number of runners per page

	// Get filtered registrations with pagination
	registrations, totalCount, err := database.GetFilteredRegistrations(seasonID, searchQuery, page, perPage)
	if err != nil {
		log.Printf("Error getting registrations: %v", err)
		http.Error(w, "Failed to retrieve registrations", http.StatusInternalServerError)
		return
	}

	// Calculate total pages
	totalPages := (totalCount + perPage - 1) / perPage
	if totalPages == 0 {
		totalPages = 1
	}

	data := PageData{
		Title:            "Run Club - Registered Runners",
		User:             username,
		Role:             role,
		Seasons:          seasons,
		ActiveSeason:     activeSeason,
		Registrations:    registrations,
		SelectedSeasonID: seasonID,
		SearchQuery:      searchQuery,
		CurrentPage:      page,
		TotalPages:       totalPages,
		TotalRunners:     totalCount,
	}

	renderTemplate(w, "runners", data)
}

func runnersExportHandler(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	seasonID := r.URL.Query().Get("season_id")
	searchQuery := template.HTMLEscapeString(r.URL.Query().Get("search"))

	// Get registrations without pagination
	registrations, err := database.GetAllRegistrations(seasonID)
	if err != nil {
		log.Printf("Error getting registrations: %v", err)
		http.Error(w, "Failed to retrieve registrations", http.StatusInternalServerError)
		return
	}

	// Filter by search query if provided
	if searchQuery != "" {
		searchTerm := strings.ToLower(searchQuery)
		var filtered []*Registration
		for _, reg := range registrations {
			if strings.Contains(strings.ToLower(reg.FirstName), searchTerm) ||
				strings.Contains(strings.ToLower(reg.LastName), searchTerm) ||
				strings.Contains(strings.ToLower(reg.Grade), searchTerm) ||
				strings.Contains(strings.ToLower(reg.Teacher), searchTerm) ||
				strings.Contains(strings.ToLower(reg.ParentEmail), searchTerm) {
				filtered = append(filtered, reg)
			}
		}
		registrations = filtered
	}

	// Set headers for CSV download
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=runners.csv")

	// Create CSV writer
	csvWriter := csv.NewWriter(w)
	defer csvWriter.Flush()

	// Write header row
	header := []string{
		"ID", "First Name", "Last Name", "Grade", "Teacher", "Gender",
		"Parent Contact", "Backup Contact", "Parent Email", "Season", "Registered On",
	}
	if err := csvWriter.Write(header); err != nil {
		log.Printf("Error writing CSV header: %v", err)
		http.Error(w, "Failed to generate CSV", http.StatusInternalServerError)
		return
	}

	// Write data rows
	for _, reg := range registrations {
		seasonName := "N/A"
		if reg.Season != nil {
			seasonName = reg.Season.Name
		}

		row := []string{
			reg.ID,
			reg.FirstName,
			reg.LastName,
			reg.Grade,
			reg.Teacher,
			reg.Gender,
			reg.ParentContactNumber,
			reg.BackupContactNumber,
			reg.ParentEmail,
			seasonName,
			reg.RegisteredAt.Format("2006-01-02"),
		}

		if err := csvWriter.Write(row); err != nil {
			log.Printf("Error writing CSV row: %v", err)
			http.Error(w, "Failed to generate CSV", http.StatusInternalServerError)
			return
		}
	}
}

func badgesHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "run-club-session")
	username := session.Values["username"].(string)
	role := session.Values["role"].(string)

	// Get all seasons
	seasons, err := database.GetAllSeasons()
	if err != nil {
		log.Printf("Error getting seasons: %v", err)
		http.Error(w, "Failed to retrieve seasons", http.StatusInternalServerError)
		return
	}

	// Get active season
	activeSeason, hasActiveSeason, err := database.GetActiveSeason()
	if err != nil {
		log.Printf("Error getting active season: %v", err)
	}

	// Parse query parameters
	seasonID := r.URL.Query().Get("season_id")

	// Default to active season if no season_id is provided
	if seasonID == "" && hasActiveSeason {
		seasonID = activeSeason.ID
	}

	// Make sure we HTML-escape the search query for template safety
	searchQuery := template.HTMLEscapeString(r.URL.Query().Get("search"))

	pageStr := r.URL.Query().Get("page")
	page := 1
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	// Number of badges per page (8 badges per sheet: 2 columns × 4 rows)
	runnersPerPage := 8

	// Get filtered registrations with pagination
	registrations, totalCount, err := database.GetFilteredRegistrations(seasonID, searchQuery, page, runnersPerPage)
	if err != nil {
		log.Printf("Error getting registrations: %v", err)
		http.Error(w, "Failed to retrieve registrations", http.StatusInternalServerError)
		return
	}

	// Calculate total pages
	totalPages := (totalCount + runnersPerPage - 1) / runnersPerPage
	if totalPages == 0 {
		totalPages = 1
	}

	data := PageData{
		Title:            "Run Club - Print Badges",
		User:             username,
		Role:             role,
		Seasons:          seasons,
		ActiveSeason:     activeSeason,
		Registrations:    registrations,
		SelectedSeasonID: seasonID,
		SearchQuery:      searchQuery,
		CurrentPage:      page,
		TotalPages:       totalPages,
		TotalRunners:     totalCount,
		RunnersPerPage:   runnersPerPage,
	}

	renderTemplate(w, "badges", data)
}

// publicRegisterHandler handles public registration for a specific season
func publicRegisterHandler(w http.ResponseWriter, r *http.Request) {
	// Get token from URL
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Missing registration token", http.StatusBadRequest)
		return
	}

	// Get season by token
	season, exists, err := database.GetSeasonByRegistrationToken(token)
	if err != nil {
		log.Printf("Error getting season by token: %v", err)
		http.Error(w, "Failed to retrieve season", http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "Invalid registration link", http.StatusNotFound)
		return
	}

	// For GET requests, show the form
	if r.Method == http.MethodGet {
		data := PageData{
			Title:        fmt.Sprintf("Run Club - Register for %s", season.Name),
			ActiveSeason: season,
			// For public registration, we don't have a logged-in user
			User: "",
			Role: "",
		}
		renderTemplate(w, "register", data)
		return
	}

	// For POST requests, process the form submission
	if r.Method == http.MethodPost {
		// Parse form data
		err := r.ParseForm()
		if err != nil {
			http.Error(w, "Error parsing form", http.StatusBadRequest)
			return
		}

		// Validate phone numbers
		parentPhone := r.FormValue("parentContactNumber")
		if !validatePhoneNumber(parentPhone) {
			http.Error(w, "Invalid parent/guardian contact number. Please use format: 123-456-7890", http.StatusBadRequest)
			return
		}
		
		backupPhone := r.FormValue("backupContactNumber")
		if backupPhone != "" && !validatePhoneNumber(backupPhone) {
			http.Error(w, "Invalid backup contact number. Please use format: 123-456-7890", http.StatusBadRequest)
			return
		}

		// Create a new registration
		reg := &Registration{
			ID:                  uuid.New().String(),
			SeasonID:            &season.ID,
			FirstName:           r.FormValue("firstName"),
			LastName:            r.FormValue("lastName"),
			Grade:               r.FormValue("grade"),
			Teacher:             r.FormValue("teacher"),
			Gender:              r.FormValue("gender"),
			TshirtSize:          r.FormValue("tshirtSize"),
			ParentFirstName:     r.FormValue("parentFirstName"),
			ParentLastName:      r.FormValue("parentLastName"),
			ParentContactNumber: r.FormValue("parentContactNumber"),
			BackupContactNumber: r.FormValue("backupContactNumber"),
			ParentEmail:         r.FormValue("parentEmail"),
			DismissalMethod:     r.FormValue("dismissalMethod"),
			Allergies:           r.FormValue("allergies"),
			MedicalInfo:         r.FormValue("medicalInfo"),
			RegisteredAt:        time.Now(),
			Season:              season,
		}

		// Save the registration to the database
		err = database.SaveRegistration(reg)
		if err != nil {
			log.Printf("Error saving registration: %v", err)
			http.Error(w, "Failed to save registration", http.StatusInternalServerError)
			return
		}

		// Redirect to public success page with token
		http.Redirect(w, r, fmt.Sprintf("/public/success?id=%s&token=%s", reg.ID, token), http.StatusSeeOther)
		return
	}

	// Method not allowed for other HTTP methods
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// publicSuccessHandler shows success page for public registrations
func publicSuccessHandler(w http.ResponseWriter, r *http.Request) {
	// Get the registration ID and token from URL query parameters
	id := r.URL.Query().Get("id")
	token := r.URL.Query().Get("token")

	if id == "" || token == "" {
		http.Error(w, "Missing parameters", http.StatusBadRequest)
		return
	}

	// Verify token is valid
	season, exists, err := database.GetSeasonByRegistrationToken(token)
	if err != nil || !exists {
		http.Error(w, "Invalid registration link", http.StatusNotFound)
		return
	}

	// Look up the registration
	reg, exists, err := database.GetRegistration(id)
	if err != nil {
		log.Printf("Error getting registration: %v", err)
		http.Error(w, "Failed to retrieve registration", http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "Registration not found", http.StatusNotFound)
		return
	}

	// Verify registration belongs to the season
	if reg.SeasonID == nil || *reg.SeasonID != season.ID {
		http.Error(w, "Registration not found", http.StatusNotFound)
		return
	}

	// Render the success page with registration details
	data := PageData{
		Title:        "Run Club - Registration Successful",
		Registration: reg,
		// For public registration, we don't have a logged-in user
		User: "",
		Role: "",
	}

	renderTemplate(w, "success", data)
}

func renderTemplate(w http.ResponseWriter, name string, data PageData) {
	tmpl, ok := templates[name]
	if !ok {
		log.Printf("Template %s not found", name)
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}

	err := tmpl.Execute(w, data)
	if err != nil {
		log.Printf("Error executing template %s: %v", name, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
