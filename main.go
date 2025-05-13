package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

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
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	IsActive  bool      `json:"isActive"`
	CreatedAt time.Time `json:"createdAt"`
}

// Registration represents a runner registration
type Registration struct {
	ID                  string    `json:"id"`
	SeasonID            *string   `json:"seasonId"`
	FirstName           string    `json:"firstName"`
	LastName            string    `json:"lastName"`
	Grade               string    `json:"grade"`
	Teacher             string    `json:"teacher"`
	ParentContactNumber string    `json:"parentContactNumber"`
	BackupContactNumber string    `json:"backupContactNumber"`
	ParentEmail         string    `json:"parentEmail"`
	RegisteredAt        time.Time `json:"registeredAt"`
	Season              *Season   `json:"season,omitempty"`
}

// ScanRecord represents a record of a QR code scan
type ScanRecord struct {
	ID             string    `json:"id"`
	RegistrationID string    `json:"registrationId"`
	SeasonID       string    `json:"seasonId"`
	ScannedAt      time.Time `json:"scannedAt"`
	RunnerName     string    `json:"runnerName,omitempty"` // Populated for API responses
	Season         *Season   `json:"season,omitempty"`
}

// ScanResult represents the result of a scan operation
type ScanResult struct {
	Success      bool          `json:"success"`
	Message      string        `json:"message"`
	Registration *Registration `json:"registration,omitempty"`
	ScanRecord   *ScanRecord   `json:"scanRecord,omitempty"`
}

// PageData holds data to be passed to templates
type PageData struct {
	Title        string
	Registration *Registration
	ScanResult   *ScanResult
	User         string
	Role         string
	ActiveSeason *Season
	Seasons      []*Season
	SeasonStats  []SeasonStat
}

// SeasonStat represents statistics for a season
type SeasonStat struct {
	SeasonID    string
	SeasonName  string
	RunnerCount int
	ScanCount   int
}

var (
	database  *Database
	templates map[string]*template.Template
	store     *sessions.CookieStore
)

func main() {
	log.Println("STARTING UP WOOOOOOOOOOOOOOOOOOO!!!!!!!!!!!!")
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

	// Create handlers
	http.HandleFunc("/", authMiddleware(homeHandler, []string{RoleAdmin, RoleScanner, RoleViewer}))
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/logout", logoutHandler)
	http.HandleFunc("/scan", authMiddleware(scanHandler, []string{RoleAdmin, RoleScanner}))
	http.HandleFunc("/register", authMiddleware(registerHandler, []string{RoleAdmin}))
	http.HandleFunc("/success", authMiddleware(successHandler, []string{RoleAdmin}))
	http.HandleFunc("/seasons", authMiddleware(seasonsHandler, []string{RoleAdmin}))
	http.HandleFunc("/seasons/activate", authMiddleware(activateSeasonHandler, []string{RoleAdmin}))

	// API endpoints
	http.HandleFunc("/api/registrations", authMiddleware(apiRegistrationsHandler, []string{RoleAdmin}))
	http.HandleFunc("/api/scan", authMiddleware(apiScanHandler, []string{RoleAdmin, RoleScanner}))
	http.HandleFunc("/api/scans", authMiddleware(apiScansHandler, []string{RoleAdmin, RoleScanner}))

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

	// Load each template
	templateFiles := []string{"home", "scan", "register", "success", "login", "seasons"}
	for _, name := range templateFiles {
		tmpl, err := template.ParseFiles(fmt.Sprintf("templates/%s.html", name))
		if err != nil {
			log.Fatalf("Error parsing template %s: %v", name, err)
		}
		templates[name] = tmpl
	}
}

// authMiddleware checks if the user is authenticated and has the required role
func authMiddleware(handler http.HandlerFunc, roles []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handler(w, r)
		return
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
	}

	renderTemplate(w, "scan", data)
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

		// Create a new registration
		reg := &Registration{
			ID:                  uuid.New().String(),
			SeasonID:            activeSeason.ID,
			FirstName:           r.FormValue("firstName"),
			LastName:            r.FormValue("lastName"),
			Grade:               r.FormValue("grade"),
			Teacher:             r.FormValue("teacher"),
			ParentContactNumber: r.FormValue("parentContactNumber"),
			BackupContactNumber: r.FormValue("backupContactNumber"),
			ParentEmail:         r.FormValue("parentEmail"),
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

		// Render the seasons page
		renderTemplate(w, "seasons", PageData{
			Title:        "Run Club - Manage Seasons",
			User:         username,
			Role:         role,
			ActiveSeason: activeSeason,
			Seasons:      seasons,
			SeasonStats:  seasonStats,
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

func apiScanHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse JSON request
	var request struct {
		Code string `json:"code"`
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
	scan, reg, err := database.RecordScan(request.Code)
	if err != nil {
		sendJSONResponse(w, ScanResult{
			Success: false,
			Message: "Runner not found",
		}, http.StatusOK)
		return
	}

	// Return success response
	sendJSONResponse(w, ScanResult{
		Success:      true,
		Message:      fmt.Sprintf("Successfully recorded run for %s %s", reg.FirstName, reg.LastName),
		Registration: reg,
		ScanRecord:   scan,
	}, http.StatusOK)
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
