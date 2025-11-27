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

    // Create change password modal if it doesn't exist
    function createChangePasswordModal() {
        if (document.getElementById("changePasswordModal")) {
            return; // Already exists
        }
        
        const modal = document.createElement("div");
        modal.id = "changePasswordModal";
        modal.className = "modal";
        modal.setAttribute("role", "dialog");
        modal.setAttribute("aria-modal", "true");
        modal.setAttribute("aria-labelledby", "changePasswordTitle");
        modal.style.display = "none";
        
        modal.innerHTML = `
            <div class="modal-content">
                <div class="modal-header">
                    <h3 id="changePasswordTitle">Change Password</h3>
                    <button class="close" id="closeChangePasswordModal" aria-label="Close">&times;</button>
                </div>
                <form id="changePasswordForm">
                    <div class="form-group">
                        <label for="currentPassword">Current Password:</label>
                        <input type="password" id="currentPassword" name="current_password" required>
                    </div>
                    <div class="form-group">
                        <label for="newPassword">New Password:</label>
                        <input type="password" id="newPassword" name="new_password" required minlength="8">
                    </div>
                    <div class="form-group">
                        <label for="confirmPassword">Confirm New Password:</label>
                        <input type="password" id="confirmPassword" name="confirm_password" required minlength="8">
                    </div>
                    <div id="changePasswordError" class="error-message" style="display: none; color: #dc3545; margin-bottom: 1rem;"></div>
                    <div class="modal-buttons">
                        <button type="button" class="btn btn-secondary" id="cancelChangePassword">Cancel</button>
                        <button type="submit" class="btn btn-primary">Change Password</button>
                    </div>
                </form>
            </div>
        `;
        
        document.body.appendChild(modal);
        
        // Set up event listeners for the new modal
        document.getElementById("closeChangePasswordModal").addEventListener("click", hideChangePasswordModal);
        document.getElementById("cancelChangePassword").addEventListener("click", hideChangePasswordModal);
        
        document.getElementById("changePasswordForm").addEventListener("submit", async function(e) {
            e.preventDefault();
            const currentPassword = document.getElementById("currentPassword").value;
            const newPassword = document.getElementById("newPassword").value;
            const confirmPassword = document.getElementById("confirmPassword").value;
            
            if (newPassword !== confirmPassword) {
                showPasswordChangeError("New passwords do not match");
                return;
            }
            
            await handlePasswordChange(currentPassword, newPassword);
        });
    }

    // Show change password modal
    function showChangePasswordModal() {
        createChangePasswordModal();
        
        const modal = document.getElementById("changePasswordModal");
        if (modal) {
            modal.style.display = "flex";
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
            // Focus first input
            const firstInput = document.getElementById("currentPassword");
            if (firstInput) {
                firstInput.focus();
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
        // User menu dropdown button (navbar style)
        const dropdownBtn = document.querySelector('.dropdown-btn');
        const dropdownContent = document.querySelector('.dropdown-content');
        
        if (dropdownBtn && dropdownContent) {
            dropdownBtn.addEventListener('click', (e) => {
                e.preventDefault();
                e.stopPropagation();
                const isExpanded = dropdownBtn.getAttribute('aria-expanded') === 'true';
                dropdownBtn.setAttribute('aria-expanded', !isExpanded);
                dropdownContent.classList.toggle('show');
            });

            // Close dropdown on outside click
            document.addEventListener('click', (e) => {
                if (!dropdownBtn.contains(e.target) && !dropdownContent.contains(e.target)) {
                    dropdownBtn.setAttribute('aria-expanded', 'false');
                    dropdownContent.classList.remove('show');
                }
            });
        }

        // Logout button (supports both ID formats)
        const logoutBtn = document.getElementById("logoutBtn") || document.getElementById("logoutButton");
        if (logoutBtn) {
            logoutBtn.addEventListener("click", function(e) {
                e.preventDefault();
                handleLogout();
            });
        }

        // Change password button (supports both ID formats)
        const changePasswordBtn = document.getElementById("changePasswordBtn") || document.getElementById("changePasswordButton");
        if (changePasswordBtn) {
            changePasswordBtn.addEventListener("click", function(e) {
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
            window.location.href = "/dashboard.html";
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