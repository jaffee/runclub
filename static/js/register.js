document.addEventListener('DOMContentLoaded', () => {
    const form = document.getElementById('register-form');
    
    if (form) {
        // Add form validation
        form.addEventListener('submit', (e) => {
            // Form will be submitted to the server
            console.log('Submitting registration form...');
        });
    }
});