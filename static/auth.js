document.addEventListener("DOMContentLoaded", async () => {
  const loginForm = document.getElementById("loginForm");
  const registerForm = document.getElementById("registerForm");
  const loginFormElement = document.getElementById("loginFormElement");
  const registerFormElement = document.getElementById("registerFormElement");
  const showRegisterLink = document.getElementById("showRegister");
  const showLoginLink = document.getElementById("showLogin");
  const statusDiv = document.getElementById("status");
  const oidcSection = document.getElementById("oidcProviders");

  // Check if user is already authenticated
  await checkAuthStatus();

  // Check OIDC providers and show if available
  await checkOIDCProviders();

  // Form switching
  showRegisterLink.addEventListener("click", (e) => {
    e.preventDefault();
    showRegistrationForm();
  });

  showLoginLink.addEventListener("click", (e) => {
    e.preventDefault();
    showLoginForm();
  });

  // Login form submission
  loginFormElement.addEventListener("submit", async (e) => {
    e.preventDefault();
    await handleLogin(e);
  });

  // Registration form submission
  registerFormElement.addEventListener("submit", async (e) => {
    e.preventDefault();
    await handleRegistration(e);
  });

  // Password confirmation validation
  const confirmPasswordField = document.getElementById("confirmPassword");
  const passwordField = document.getElementById("registerPassword");

  confirmPasswordField.addEventListener("input", () => {
    validatePasswordMatch();
  });

  passwordField.addEventListener("input", () => {
    validatePasswordMatch();
  });

  function showRegistrationForm() {
    loginForm.style.display = "none";
    registerForm.style.display = "block";
    clearStatus();
    
    // Focus first input in registration form
    const firstInput = registerForm.querySelector('input');
    if (firstInput) {
      firstInput.focus();
    }
    
    // Announce form change
    if (window.AccessibilityModule) {
      window.AccessibilityModule.announce('Registration form opened', 'polite');
    }
  }

  function showLoginForm() {
    registerForm.style.display = "none";
    loginForm.style.display = "block";
    clearStatus();
    
    // Focus first input in login form
    const firstInput = loginForm.querySelector('input');
    if (firstInput) {
      firstInput.focus();
    }
    
    // Announce form change
    if (window.AccessibilityModule) {
      window.AccessibilityModule.announce('Login form opened', 'polite');
    }
  }

  function validatePasswordMatch() {
    const password = passwordField.value;
    const confirmPassword = confirmPasswordField.value;
    const errorElement = document.getElementById('confirmPassword-error');

    if (confirmPassword && password !== confirmPassword) {
      confirmPasswordField.setCustomValidity("Passwords do not match");
      confirmPasswordField.setAttribute('aria-invalid', 'true');
      showFieldError('confirmPassword', 'Passwords do not match');
    } else {
      confirmPasswordField.setCustomValidity("");
      confirmPasswordField.setAttribute('aria-invalid', 'false');
      hideFieldError('confirmPassword');
    }
  }

  // Show field-specific error
  function showFieldError(fieldId, message) {
    const errorElement = document.getElementById(`${fieldId}-error`);
    if (errorElement) {
      errorElement.textContent = message;
      errorElement.style.display = 'block';
    }
  }

  // Hide field-specific error
  function hideFieldError(fieldId) {
    const errorElement = document.getElementById(`${fieldId}-error`);
    if (errorElement) {
      errorElement.style.display = 'none';
      errorElement.textContent = '';
    }
  }

  // Clear all field errors
  function clearAllFieldErrors() {
    const errorElements = document.querySelectorAll('.error-message');
    errorElements.forEach(element => {
      element.style.display = 'none';
      element.textContent = '';
    });
    
    // Reset aria-invalid attributes
    const inputs = document.querySelectorAll('input');
    inputs.forEach(input => {
      input.setAttribute('aria-invalid', 'false');
    });
  }

  async function handleLogin(e) {
    const formData = new FormData(e.target);
    const username = formData.get("username");
    const password = formData.get("password");

    // Clear previous errors
    clearAllFieldErrors();
    clearStatus();

    if (!username || !password) {
      showStatus("Please fill in all fields", "error");
      
      // Focus first empty field
      if (!username) {
        document.getElementById('loginUsername').focus();
        showFieldError('loginUsername', 'Username is required');
      } else if (!password) {
        document.getElementById('loginPassword').focus();
        showFieldError('loginPassword', 'Password is required');
      }
      return;
    }

    const submitButton = e.target.querySelector('button[type="submit"]');
    setButtonLoading(submitButton, true);
    
    // Update button text for screen readers
    const originalText = submitButton.textContent;
    submitButton.setAttribute('aria-label', 'Signing in, please wait');

    try {
      const response = await fetch("/api/auth/login", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ username, password }),
      });

      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(errorText || `HTTP error! status: ${response.status}`);
      }

      const user = await response.json();
      
      // Set user in shared auth module
      window.authModule.setCurrentUser(user);
      
      showStatus(`Welcome back, ${user.username}!`, "success");

      // Redirect to main application after successful login
      setTimeout(() => {
        window.location.href = "/";
      }, 1500);
    } catch (error) {
      console.error("Login error:", error);
      showStatus(error.message || "Login failed", "error");
    } finally {
      setButtonLoading(submitButton, false);
      // Restore original button label
      submitButton.setAttribute('aria-label', originalText);
    }
  }

  async function handleRegistration(e) {
    const formData = new FormData(e.target);
    const username = formData.get("username");
    const email = formData.get("email");
    const password = formData.get("password");
    const confirmPassword = formData.get("confirmPassword");

    if (!username || !password || !confirmPassword) {
      showStatus("Please fill in all required fields", "error");
      return;
    }

    if (password !== confirmPassword) {
      showStatus("Passwords do not match", "error");
      return;
    }

    if (password.length < 6) {
      showStatus("Password must be at least 6 characters long", "error");
      return;
    }

    const submitButton = e.target.querySelector('button[type="submit"]');
    setButtonLoading(submitButton, true);

    try {
      const response = await fetch("/api/auth/register", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ username, email: email || undefined, password }),
      });

      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(errorText || `HTTP error! status: ${response.status}`);
      }

      const user = await response.json();
      
      // Set user in shared auth module
      window.authModule.setCurrentUser(user);
      
      showStatus(
        `Account created successfully! Welcome, ${user.username}!`,
        "success",
      );

      // Redirect to main application after successful registration
      setTimeout(() => {
        window.location.href = "/";
      }, 1500);
    } catch (error) {
      console.error("Registration error:", error);
      if (error.message.includes("already exists")) {
        showStatus(error.message, "error");
      } else if (error.message.includes("disabled")) {
        showStatus("Registration is currently disabled", "error");
      } else {
        showStatus(error.message || "Registration failed", "error");
      }
    } finally {
      setButtonLoading(submitButton, false);
    }
  }

  async function checkAuthStatus() {
    // Use shared auth module to check authentication
    const user = await window.authModule.checkAuthenticationStatus();
    if (user) {
      // User is already authenticated, redirect to main app
      window.location.href = "/";
    }
  }

  async function checkOIDCProviders() {
    try {
      // Check if OIDC providers are configured by trying to access the endpoints
      // We'll make this a simple check by seeing if the links would work

      // For now, we'll check the config or make a simple request
      // This is a placeholder - in a real implementation, you'd have an endpoint
      // that returns the available OIDC providers

      // Show OIDC section if we detect any providers are available
      // This is a simplified check - you'd want to make this more robust
      const googleBtn = oidcSection.querySelector(".google-btn");
      const microsoftBtn = oidcSection.querySelector(".microsoft-btn");

      // For demo purposes, let's show Google and Microsoft if they might be configured
      // In a real implementation, you'd check the server configuration
      const hasProviders = false; // Set to true when OIDC is properly configured

      if (hasProviders) {
        oidcSection.style.display = "block";
        googleBtn.style.display = "flex";
        microsoftBtn.style.display = "flex";
      }
    } catch (error) {
      console.log("OIDC providers not available");
    }
  }

  function setButtonLoading(button, loading) {
    if (loading) {
      button.disabled = true;
      button.classList.add("loading");
    } else {
      button.disabled = false;
      button.classList.remove("loading");
    }
  }

  function showStatus(message, type) {
    statusDiv.textContent = message;
    statusDiv.className = `status ${type}`;
    statusDiv.style.display = "block";
    
    // Announce to screen readers
    if (window.AccessibilityModule) {
      const priority = type === 'error' ? 'assertive' : 'polite';
      window.AccessibilityModule.announce(message, priority);
    }

    // Auto-hide success messages after 3 seconds
    if (type === "success") {
      setTimeout(clearStatus, 3000);
    }
  }

  function clearStatus() {
    statusDiv.textContent = "";
    statusDiv.className = "status";
    statusDiv.style.display = "none";
  }

  // Handle URL parameters for OIDC callbacks or error messages
  const urlParams = new URLSearchParams(window.location.search);
  const error = urlParams.get("error");
  const message = urlParams.get("message");

  if (error) {
    showStatus(error, "error");
  } else if (message) {
    showStatus(message, "info");
  }

  // Check if registration is allowed by trying to access the register endpoint info
  // This is a simple way to hide the registration link if it's disabled
  try {
    // We could make a HEAD request to check if registration is enabled
    // For now, we'll leave the registration form visible
  } catch (error) {
    // Hide registration link if not allowed
    showRegisterLink.style.display = "none";
  }
});
