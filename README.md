# Run Club

A web-based application for tracking student participation in a school running club.

## Features

- Home page with navigation
- QR code scanning page for recording student runs
- Student registration form
- Responsive design that works on both desktop and mobile

## Project Structure

```
/
├── main.go           # Go web server
├── go.mod            # Go module file
├── Makefile          # Build and run commands
├── templates/        # HTML templates
│   ├── home.html     # Home page template
│   ├── scan.html     # QR scanner page template
│   ├── register.html # Registration page template
│   └── success.html  # Registration success page template
└── static/           # Static assets
    ├── css/
    │   └── style.css # Stylesheets
    └── js/
        ├── scanner.js  # QR scanner functionality
        └── register.js # Registration form functionality
```

## Running the Application

```bash
go build .
./runclub
```

The server will start at http://localhost:8080 by default.

### Command Line Options

- `-port` - Specify a custom port (default: 8080)

Example:
```bash
./runclub -port 3000
```

## Deploying

```bash
fly deploy
```

## Pages

- **Home Page** (/) - Main navigation page with links to scan and register
- **Scan Page** (/scan) - QR code scanner for recording student runs
- **Register Page** (/register) - Registration form for new participants
- **Success Page** (/success) - Displays registration details and QR code after successful registration

## API Endpoints

- **GET /api/registrations** - Get all registered students
- **POST /api/scan** - Record a run by scanning a student's QR code
- **GET /api/scans** - Get all recorded runs
- **GET /api/scans?registration_id={id}** - Get all runs for a specific student

## How It Works

1. Register students through the registration form
2. Each student receives a unique QR code
3. When a student completes a run, scan their QR code
4. The system records the run and displays the student's information


## Technologies Used

- **Backend**: Go (web server, HTML templating)
- **Frontend**: HTML5, CSS3, JavaScript
- **Libraries**: jsQR (QR code detection), QRCode.js (QR code generation)
- **Data Storage**: SQLite
- **Deployment**: fly.io