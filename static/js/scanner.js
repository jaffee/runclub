document.addEventListener('DOMContentLoaded', () => {
    const video = document.getElementById('video');
    const canvas = document.getElementById('canvas');
    const resultElement = document.getElementById('result');
    const resultMessage = document.getElementById('result-message');
    const scanHistory = document.getElementById('scan-history');
    const canvasContext = canvas.getContext('2d');
    
    let scanning = false;
    let recentScans = [];
    let selectedTrackId = null;
    
    // Track selection handling
    const trackButtons = document.querySelectorAll('.track-btn');
    const selectedTrackName = document.getElementById('selected-track-name');
    
    // Auto-select default track if available
    const defaultTrack = document.querySelector('.track-btn.default');
    if (defaultTrack) {
        defaultTrack.click();
    }
    
    trackButtons.forEach(button => {
        button.addEventListener('click', () => {
            // Remove active class from all buttons
            trackButtons.forEach(btn => btn.classList.remove('active'));
            
            // Add active class to clicked button
            button.classList.add('active');
            
            // Update selected track
            selectedTrackId = button.dataset.trackId;
            const trackName = button.dataset.trackName;
            const trackMiles = button.dataset.trackMiles;
            selectedTrackName.textContent = `${trackName} (${trackMiles} miles)`;
        });
    });
    
    // Auto-click default track if it exists
    if (defaultTrack) {
        defaultTrack.click();
    }

    // Check if browser supports getUserMedia
    if (navigator.mediaDevices && navigator.mediaDevices.getUserMedia) {
        startCamera();
    } else {
        updateResultMessage('Sorry, your browser does not support accessing the camera.', 'alert-danger');
    }

    async function startCamera() {
        try {
            const stream = await navigator.mediaDevices.getUserMedia({ 
                video: { 
                    facingMode: 'environment',
                    width: { ideal: 1280 },
                    height: { ideal: 720 }
                } 
            });
            
            video.srcObject = stream;
            video.setAttribute('playsinline', true); // Required for iOS
            video.play();
            scanning = true;
            requestAnimationFrame(scanQRCode);
        } catch (error) {
            console.error('Error accessing camera:', error);
            updateResultMessage(`Error accessing camera: ${error.message}`, 'alert-danger');
        }
    }

    function scanQRCode() {
        if (!scanning) return;

        if (video.readyState === video.HAVE_ENOUGH_DATA) {
            // Set canvas size to match video
            canvas.width = video.videoWidth;
            canvas.height = video.videoHeight;
            
            // Calculate the center area to scan
            // Use a much larger scan area (400x400) than the visual indicator (200x200) for better detection
            const scanSize = 400;
            const centerX = canvas.width / 2;
            const centerY = canvas.height / 2;
            const startX = Math.max(0, centerX - scanSize / 2);
            const startY = Math.max(0, centerY - scanSize / 2);
            
            // Draw current video frame to canvas
            canvasContext.drawImage(video, 0, 0, canvas.width, canvas.height);
            
            // Get image data only from the center scanning area
            const imageData = canvasContext.getImageData(startX, startY, scanSize, scanSize);
            
            // Scan for QR code only in the center area
            const code = jsQR(imageData.data, imageData.width, imageData.height, {
                inversionAttempts: 'dontInvert',
            });
            
            if (code) {
                // QR code found
                console.log('QR code detected:', code.data);
                resultElement.textContent = code.data;
                
                // Adjust QR code location coordinates to account for the scanning area offset
                const adjustedLocation = {
                    topLeftCorner: { x: code.location.topLeftCorner.x + startX, y: code.location.topLeftCorner.y + startY },
                    topRightCorner: { x: code.location.topRightCorner.x + startX, y: code.location.topRightCorner.y + startY },
                    bottomRightCorner: { x: code.location.bottomRightCorner.x + startX, y: code.location.bottomRightCorner.y + startY },
                    bottomLeftCorner: { x: code.location.bottomLeftCorner.x + startX, y: code.location.bottomLeftCorner.y + startY }
                };
                
                // Highlight QR code location
                drawQRCodeOutline(adjustedLocation);
                
                // Process the QR code if it looks like a UUID
                processQRCode(code.data);
                
                // Pause scanning for a moment to prevent duplicate scans
                scanning = false;
                setTimeout(() => {
                    scanning = true;
                    requestAnimationFrame(scanQRCode);
                }, 2000);
            } else {
                // Continue scanning
                requestAnimationFrame(scanQRCode);
            }
        } else {
            // Continue scanning
            requestAnimationFrame(scanQRCode);
        }
    }

    function drawQRCodeOutline(location) {
        // Draw outline around detected QR code
        canvasContext.beginPath();
        canvasContext.moveTo(location.topLeftCorner.x, location.topLeftCorner.y);
        canvasContext.lineTo(location.topRightCorner.x, location.topRightCorner.y);
        canvasContext.lineTo(location.bottomRightCorner.x, location.bottomRightCorner.y);
        canvasContext.lineTo(location.bottomLeftCorner.x, location.bottomLeftCorner.y);
        canvasContext.lineTo(location.topLeftCorner.x, location.topLeftCorner.y);
        canvasContext.lineWidth = 4;
        canvasContext.strokeStyle = '#FF3B58';
        canvasContext.stroke();
    }

    function updateResultMessage(message, alertType) {
        resultMessage.textContent = message;
        resultMessage.className = `alert ${alertType}`;
    }

    async function processQRCode(code) {
        try {
            // Check if QR code contains a valid UUID format
            const isUUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(code);
            
            if (!isUUID) {
                updateResultMessage('Not a valid runner ID', 'alert-warning');
                return;
            }
            
            updateResultMessage('Processing...', 'alert-info');
            
            // Send the code to the server for validation and recording
            const requestBody = { code };
            if (selectedTrackId) {
                requestBody.trackId = selectedTrackId;
            }
            
            const response = await fetch('/api/scan', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify(requestBody),
            });
            
            const result = await response.json();
            
            if (result.success) {
                // Runner found and scan recorded
                let message = result.message;
                
                // Add lap time and pace info if available
                if (result.lapTime !== undefined && result.lapTime !== null) {
                    const lapMinutes = Math.floor(result.lapTime);
                    const lapSeconds = Math.round((result.lapTime - lapMinutes) * 60);
                    message += ` | Lap time: ${lapMinutes}:${lapSeconds.toString().padStart(2, '0')}`;
                    
                    if (result.pace !== undefined && result.pace !== null) {
                        const paceMinutes = Math.floor(result.pace);
                        const paceSeconds = Math.round((result.pace - paceMinutes) * 60);
                        message += ` | Pace: ${paceMinutes}:${paceSeconds.toString().padStart(2, '0')}/mile`;
                    }
                }
                
                updateResultMessage(message, 'alert-success');
                
                // Add to recent scans list
                addToRecentScans({
                    id: result.scanRecord.id,
                    studentName: `${result.registration.firstName} ${result.registration.lastName}`,
                    grade: result.registration.grade,
                    teacher: result.registration.teacher,
                    seasonName: result.scanRecord.season ? result.scanRecord.season.name : 'Unknown Season',
                    trackName: result.scanRecord.track ? result.scanRecord.track.name : 'No Track',
                    trackDistance: result.scanRecord.track ? result.scanRecord.track.distanceMiles : null,
                    scannedAt: new Date(),
                    lapTime: result.lapTime,
                    pace: result.pace,
                });
            } else {
                // QR code was valid UUID but runner not found
                updateResultMessage(result.message, 'alert-warning');
            }
        } catch (error) {
            console.error('Error processing QR code:', error);
            updateResultMessage('Error processing QR code', 'alert-danger');
        }
    }

    function addToRecentScans(scan) {
        // Add scan to recent scans array (limit to 10)
        recentScans.unshift(scan);
        if (recentScans.length > 10) {
            recentScans.pop();
        }
        
        // Update the scan history display
        updateScanHistory();
    }

    function updateScanHistory() {
        // Clear the scan history
        scanHistory.innerHTML = '';
        
        // If no scans, show empty message
        if (recentScans.length === 0) {
            const emptyMessage = document.createElement('p');
            emptyMessage.className = 'empty-history';
            emptyMessage.textContent = 'No scans yet';
            scanHistory.appendChild(emptyMessage);
            return;
        }
        
        // Add each scan to the history
        recentScans.forEach(scan => {
            const scanItem = document.createElement('div');
            scanItem.className = 'scan-item';
            
            const scanDetails = document.createElement('div');
            scanDetails.className = 'scan-details';
            
            const scanName = document.createElement('div');
            scanName.className = 'scan-name';
            scanName.textContent = scan.studentName;
            
            const scanInfo = document.createElement('div');
            scanInfo.className = 'scan-info';
            scanInfo.textContent = `Grade: ${scan.grade}, Teacher: ${scan.teacher}`;

            const seasonInfo = document.createElement('div');
            seasonInfo.className = 'season-info';
            seasonInfo.textContent = `Season: ${scan.seasonName}`;
            
            const trackInfo = document.createElement('div');
            trackInfo.className = 'track-info';
            if (scan.trackDistance) {
                trackInfo.textContent = `Track: ${scan.trackName} (${scan.trackDistance} miles)`;
            } else {
                trackInfo.textContent = `Track: ${scan.trackName}`;
            }

            const scanTime = document.createElement('div');
            scanTime.className = 'scan-time';
            scanTime.textContent = formatTime(scan.scannedAt);
            
            scanDetails.appendChild(scanName);
            scanDetails.appendChild(scanInfo);
            scanDetails.appendChild(seasonInfo);
            scanDetails.appendChild(trackInfo);
            
            // Add lap time and pace info if available
            if (scan.lapTime !== undefined && scan.lapTime !== null) {
                const performanceInfo = document.createElement('div');
                performanceInfo.className = 'performance-info';
                
                const lapMinutes = Math.floor(scan.lapTime);
                const lapSeconds = Math.round((scan.lapTime - lapMinutes) * 60);
                let perfText = `Lap: ${lapMinutes}:${lapSeconds.toString().padStart(2, '0')}`;
                
                if (scan.pace !== undefined && scan.pace !== null) {
                    const paceMinutes = Math.floor(scan.pace);
                    const paceSeconds = Math.round((scan.pace - paceMinutes) * 60);
                    perfText += ` | Pace: ${paceMinutes}:${paceSeconds.toString().padStart(2, '0')}/mile`;
                }
                
                performanceInfo.textContent = perfText;
                performanceInfo.style.fontWeight = 'bold';
                performanceInfo.style.color = '#27ae60';
                scanDetails.appendChild(performanceInfo);
            }
            
            scanDetails.appendChild(scanTime);
            
            scanItem.appendChild(scanDetails);
            scanHistory.appendChild(scanItem);
        });
    }

    function formatTime(date) {
        // Format date as "Today at 2:30 PM" or "May 8 at 2:30 PM"
        const now = new Date();
        const isToday = now.toDateString() === date.toDateString();
        
        const timeStr = date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
        
        if (isToday) {
            return `Today at ${timeStr}`;
        } else {
            const month = date.toLocaleString('default', { month: 'short' });
            const day = date.getDate();
            return `${month} ${day} at ${timeStr}`;
        }
    }
});