/**
 * Shared authentication utilities
 * Eliminates duplication across app.js, dashboard.js, auth.js, admin.js, admin-users.js
 */

// Global auth state
window.authModule = (function() {
    let currentUser = null;

    // Check authentication status
    async function checkAuthenticationStatus() {
        try {
            const response = await fetch("/api/auth/me", {
                method: "GET",
                credentials: "same-origin"
            });

            if (response.ok) {
                const responseData = await response.json();
                // Extract the user data from the API response
                currentUser = responseData.data || responseData;
                return currentUser;
            } else {
                currentUser = null;
                return null;
            }
        } catch (error) {
            console.error("Failed to check authentication status:", error);
            currentUser = null;
            return null;
        }
    }

    // Get current user (from cache or fetch)
    function getCurrentUser() {
        return currentUser;
    }

    // Set current user (for use after login/registration)
    function setCurrentUser(user) {
        currentUser = user;
    }

    // Clear current user (for logout)
    function clearCurrentUser() {
        currentUser = null;
    }

    // Check if user has specific role
    function hasRole(requiredRole) {
        if (!currentUser) return false;
        
        const roleHierarchy = {
            'user': 1,
            'moderator': 2,
            'admin': 3
        };
        
        const userLevel = roleHierarchy[currentUser.role] || 0;
        const requiredLevel = roleHierarchy[requiredRole] || 999;
        
        return userLevel >= requiredLevel;
    }

    // Handle logout
    async function handleLogout() {
        try {
            const response = await fetch("/api/auth/logout", {
                method: "POST",
                credentials: "same-origin"
            });

            if (response.ok) {
                clearCurrentUser();
                window.location.href = "/auth.html";
            } else {
                console.error("Logout failed");
                // Force redirect anyway for security
                clearCurrentUser();
                window.location.href = "/auth.html";
            }
        } catch (error) {
            console.error("Logout error:", error);
            // Force redirect anyway for security
            clearCurrentUser();
            window.location.href = "/auth.html";
        }
    }

    // Show change password modal
    function showChangePasswordModal() {
        const modal = document.getElementById("changePasswordModal");
        if (modal) {
            modal.style.display = "block";
            // Clear previous form data
            const form = document.getElementById("changePasswordForm");
            if (form) {
                form.reset();
            }
            // Clear any previous errors
            const errorDiv = document.getElementById("changePasswordError");
            if (errorDiv) {
                errorDiv.style.display = "none";
            }
        }
    }

    // Hide change password modal
    function hideChangePasswordModal() {
        const modal = document.getElementById("changePasswordModal");
        if (modal) {
            modal.style.display = "none";
        }
    }

    // Handle password change
    async function handlePasswordChange(currentPassword, newPassword) {
        try {
            const response = await fetch("/api/auth/change-password", {
                method: "POST",
                headers: {
                    "Content-Type": "application/json"
                },
                credentials: "same-origin",
                body: JSON.stringify({
                    current_password: currentPassword,
                    new_password: newPassword
                })
            });

            if (response.ok) {
                hideChangePasswordModal();
                showSuccessMessage("Password changed successfully");
                return true;
            } else {
                const errorText = await response.text();
                showPasswordChangeError(errorText || "Failed to change password");
                return false;
            }
        } catch (error) {
            console.error("Password change error:", error);
            showPasswordChangeError("Network error occurred");
            return false;
        }
    }

    // Show password change error
    function showPasswordChangeError(message) {
        const errorDiv = document.getElementById("changePasswordError");
        if (errorDiv) {
            errorDiv.textContent = message;
            errorDiv.style.display = "block";
        }
    }

    // Show success message (generic utility)
    function showSuccessMessage(message) {
        // Create or update a success message element
        let successDiv = document.getElementById("successMessage");
        if (!successDiv) {
            successDiv = document.createElement("div");
            successDiv.id = "successMessage";
            successDiv.style.cssText = `
                position: fixed;
                top: 20px;
                right: 20px;
                background: #4CAF50;
                color: white;
                padding: 15px 20px;
                border-radius: 4px;
                z-index: 1000;
                box-shadow: 0 2px 10px rgba(0,0,0,0.1);
            `;
            document.body.appendChild(successDiv);
        }
        
        successDiv.textContent = message;
        successDiv.style.display = "block";
        
        // Auto-hide after 3 seconds
        setTimeout(() => {
            if (successDiv) {
                successDiv.style.display = "none";
            }
        }, 3000);
    }

    // Update user menu in the UI
    function updateUserMenu() {
        const userMenuButton = document.getElementById("userMenuButton");
        const userMenu = document.getElementById("userMenu");
        const loginSection = document.getElementById("loginSection");
        const usernameElement = document.getElementById("username");

        if (currentUser) {
            // Update username display - handle different HTML structures
            if (usernameElement) {
                usernameElement.textContent = `Welcome, ${currentUser.display_name || currentUser.username}`;
            }
            
            // Show user menu
            if (userMenuButton) {
                userMenuButton.textContent = currentUser.display_name || currentUser.username;
                userMenuButton.style.display = "block";
            }
            if (userMenu) {
                userMenu.style.display = "block";
            }
            if (loginSection) {
                loginSection.style.display = "none";
            }

            // Show/hide admin links based on role
            const adminLinks = document.querySelectorAll('.admin-only');
            adminLinks.forEach(link => {
                link.style.display = hasRole('moderator') ? 'block' : 'none';
            });
            
            // Update admin link visibility
            const adminLink = document.getElementById("adminLink");
            if (adminLink) {
                adminLink.style.display = hasRole('moderator') ? 'block' : 'none';
                adminLink.href = "/admin.html";
            }
        } else {
            // Show login section
            if (userMenuButton) {
                userMenuButton.style.display = "none";
            }
            if (userMenu) {
                userMenu.style.display = "none";
            }
            if (loginSection) {
                loginSection.style.display = "block";
            }
            if (usernameElement) {
                usernameElement.textContent = "Loading...";
            }
        }
    }

    // Initialize authentication on page load
    async function initAuth() {
        await checkAuthenticationStatus();
        updateUserMenu();
        
        // Set up event listeners if elements exist
        setupEventListeners();
    }

    // Set up common event listeners
    function setupEventListeners() {
        // User menu button
        const userMenuButton = document.getElementById("userMenuButton");
        if (userMenuButton) {
            userMenuButton.addEventListener("click", function(e) {
                e.preventDefault();
                const dropdown = document.getElementById("userDropdown");
                if (dropdown) {
                    dropdown.style.display = dropdown.style.display === "block" ? "none" : "block";
                }
            });
        }

        // Logout button
        const logoutButton = document.getElementById("logoutButton");
        if (logoutButton) {
            logoutButton.addEventListener("click", function(e) {
                e.preventDefault();
                handleLogout();
            });
        }

        // Change password button
        const changePasswordButton = document.getElementById("changePasswordButton");
        if (changePasswordButton) {
            changePasswordButton.addEventListener("click", function(e) {
                e.preventDefault();
                showChangePasswordModal();
            });
        }

        // Change password form
        const changePasswordForm = document.getElementById("changePasswordForm");
        if (changePasswordForm) {
            changePasswordForm.addEventListener("submit", async function(e) {
                e.preventDefault();
                const formData = new FormData(e.target);
                const currentPassword = formData.get("current_password");
                const newPassword = formData.get("new_password");
                
                if (currentPassword && newPassword) {
                    await handlePasswordChange(currentPassword, newPassword);
                }
            });
        }

        // Close modals when clicking outside
        window.addEventListener("click", function(event) {
            const modal = document.getElementById("changePasswordModal");
            if (modal && event.target === modal) {
                hideChangePasswordModal();
            }
        });

        // Close dropdowns when clicking outside
        window.addEventListener("click", function(event) {
            if (!event.target.matches('#userMenuButton')) {
                const dropdown = document.getElementById("userDropdown");
                if (dropdown && dropdown.style.display === "block") {
                    dropdown.style.display = "none";
                }
            }
        });
    }

    // Redirect to auth page if not authenticated
    function requireAuth() {
        if (!currentUser) {
            window.location.href = "/auth.html";
            return false;
        }
        return true;
    }

    // Redirect to access denied if insufficient role
    function requireRole(requiredRole) {
        if (!currentUser) {
            window.location.href = "/auth.html";
            return false;
        }
        
        if (!hasRole(requiredRole)) {
            window.location.href = "/welcome.html";
            return false;
        }
        
        return true;
    }

    // Public API
    return {
        checkAuthenticationStatus,
        getCurrentUser,
        setCurrentUser,
        clearCurrentUser,
        hasRole,
        handleLogout,
        showChangePasswordModal,
        hideChangePasswordModal,
        handlePasswordChange,
        showSuccessMessage,
        updateUserMenu,
        initAuth,
        requireAuth,
        requireRole
    };
})();

// Auto-initialize auth when DOM is loaded
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', window.authModule.initAuth);
} else {
    window.authModule.initAuth();
}