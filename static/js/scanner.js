document.addEventListener('DOMContentLoaded', () => {
    const video = document.getElementById('video');
    const canvas = document.getElementById('canvas');
    const resultElement = document.getElementById('result');
    const resultMessage = document.getElementById('result-message');
    const scanHistory = document.getElementById('scan-history');
    const canvasContext = canvas.getContext('2d');
    
    let scanning = false;
    let recentScans = [];

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
            
            // Draw current video frame to canvas
            canvasContext.drawImage(video, 0, 0, canvas.width, canvas.height);
            
            // Get image data for QR code scanning
            const imageData = canvasContext.getImageData(0, 0, canvas.width, canvas.height);
            
            // Scan for QR code
            const code = jsQR(imageData.data, imageData.width, imageData.height, {
                inversionAttempts: 'dontInvert',
            });
            
            if (code) {
                // QR code found
                console.log('QR code detected:', code.data);
                resultElement.textContent = code.data;
                
                // Highlight QR code location
                drawQRCodeOutline(code.location);
                
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
            const response = await fetch('/api/scan', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({ code }),
            });
            
            const result = await response.json();
            
            if (result.success) {
                // Runner found and scan recorded
                updateResultMessage(result.message, 'alert-success');
                
                // Add to recent scans list
                addToRecentScans({
                    id: result.scanRecord.id,
                    studentName: `${result.registration.firstName} ${result.registration.lastName}`,
                    grade: result.registration.grade,
                    teacher: result.registration.teacher,
                    seasonName: result.scanRecord.season ? result.scanRecord.season.name : 'Unknown Season',
                    scannedAt: new Date(),
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

            const scanTime = document.createElement('div');
            scanTime.className = 'scan-time';
            scanTime.textContent = formatTime(scan.scannedAt);
            
            scanDetails.appendChild(scanName);
            scanDetails.appendChild(scanInfo);
            scanDetails.appendChild(seasonInfo);
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