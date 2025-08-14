document.addEventListener('DOMContentLoaded', () => {
    const form = document.getElementById('register-form');
    
    if (form) {
        // Format phone number inputs as user types
        const phoneInputs = document.querySelectorAll('input[type="tel"]');
        
        phoneInputs.forEach(input => {
            input.addEventListener('input', (e) => {
                let value = e.target.value.replace(/\D/g, ''); // Remove non-digits
                
                // Format as 123-456-7890
                if (value.length > 0) {
                    if (value.length <= 3) {
                        value = value;
                    } else if (value.length <= 6) {
                        value = value.slice(0, 3) + '-' + value.slice(3);
                    } else {
                        value = value.slice(0, 3) + '-' + value.slice(3, 6) + '-' + value.slice(6, 10);
                    }
                }
                
                e.target.value = value;
            });
            
            // Validate on blur
            input.addEventListener('blur', (e) => {
                const value = e.target.value;
                const isValid = /^\d{3}-\d{3}-\d{4}$/.test(value);
                
                if (value && !isValid) {
                    e.target.setCustomValidity('Please enter a valid phone number (123-456-7890)');
                    e.target.reportValidity();
                } else {
                    e.target.setCustomValidity('');
                }
            });
        });
        
        // Add form validation
        form.addEventListener('submit', (e) => {
            const parentPhone = document.getElementById('parentContactNumber').value;
            const backupPhone = document.getElementById('backupContactNumber').value;
            
            // Validate parent phone (required)
            if (!(/^\d{3}-\d{3}-\d{4}$/.test(parentPhone))) {
                e.preventDefault();
                alert('Please enter a valid parent contact number (123-456-7890)');
                document.getElementById('parentContactNumber').focus();
                return false;
            }
            
            // Validate backup phone (optional, but if provided must be valid)
            if (backupPhone && !(/^\d{3}-\d{3}-\d{4}$/.test(backupPhone))) {
                e.preventDefault();
                alert('Please enter a valid backup contact number (123-456-7890)');
                document.getElementById('backupContactNumber').focus();
                return false;
            }
            
            console.log('Submitting registration form...');
        });
    }
});