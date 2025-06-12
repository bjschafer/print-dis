document.addEventListener("DOMContentLoaded", () => {
  // Initialize page elements first
  const form = document.getElementById("printJobForm");
  const statusDiv = document.getElementById("status");
  const spoolmanEnabled = document.getElementById("spoolmanEnabled");
  const spoolmanFields = document.getElementById("spoolmanFields");
  const manualFields = document.getElementById("manualFields");
  const manualEntrySection = manualFields.closest(".selection-option");
  const spoolmanSpool = document.getElementById("spoolmanSpool");
  const spoolmanMaterial = document.getElementById("spoolmanMaterial");

  // Set initial state
  spoolmanFields.style.display = "none";
  manualFields.style.display = "block";

  // Set up event listeners for existing HTML elements
  setupUserMenuEventListeners();

  // Check authentication after DOM is ready
  checkAuthenticationStatus();

  // URL validation function
  function isValidUrl(url) {
    try {
      new URL(url);
      return true;
    } catch (e) {
      return false;
    }
  }

  // Check if user is authenticated
  async function checkAuthenticationStatus() {
    try {
      const response = await fetch("/api/auth/me", {
        method: "GET",
        credentials: "same-origin",
      });

      if (!response.ok) {
        // User is not authenticated, redirect to auth page
        window.location.href = "/auth.html";
        return;
      }

      const user = await response.json();
      // Hide the submitter field since we now know the user
      const submitterField = document.getElementById("submitter");
      if (submitterField) {
        submitterField.closest(".form-group").style.display = "none";
        // Remove the required attribute to prevent form validation issues
        submitterField.removeAttribute("required");
        // Set a default value since we know the user
        submitterField.value = user.username;
      }

      // Store user info globally
      window.currentUser = user;

      // Update the username display now that we have the user info
      updateUsernameDisplay();

      // Show admin link if user has permissions
      updateAdminLink();
    } catch (error) {
      console.error("Auth check failed:", error);
      // Redirect to auth page on error
      window.location.href = "/auth.html";
    }
  }

  // Update username display
  function updateUsernameDisplay() {
    const usernameElement = document.getElementById("username");
    if (usernameElement && window.currentUser) {
      usernameElement.textContent = `Welcome, ${window.currentUser.username}`;
    }
  }

  // Update admin link visibility
  function updateAdminLink() {
    const adminLink = document.getElementById("adminLink");
    if (
      adminLink &&
      window.currentUser &&
      (window.currentUser.role === "admin" ||
        window.currentUser.role === "moderator")
    ) {
      adminLink.style.display = "block";
      adminLink.href = "/admin.html";
    }
  }

  // Set up event listeners for the existing HTML user menu elements
  function setupUserMenuEventListeners() {
    // Add event listeners for the existing HTML elements
    const logoutBtn = document.getElementById("logoutBtn");
    const changePasswordBtn = document.getElementById("changePasswordBtn");

    if (logoutBtn) {
      logoutBtn.addEventListener("click", handleLogout);
    }

    if (changePasswordBtn) {
      changePasswordBtn.addEventListener("click", showChangePasswordModal);
    }
  }

  // Handle logout
  async function handleLogout(e) {
    e.preventDefault();

    try {
      const response = await fetch("/api/auth/logout", {
        method: "POST",
        credentials: "same-origin",
      });

      if (response.ok) {
        window.location.href = "/auth.html";
      } else {
        throw new Error("Logout failed");
      }
    } catch (error) {
      console.error("Logout error:", error);
      // Force redirect even if logout failed
      window.location.href = "/auth.html";
    }
  }

  // Show change password modal
  function showChangePasswordModal(e) {
    e.preventDefault();

    const modal = document.createElement("div");
    modal.className = "modal";
    modal.innerHTML = `
      <div class="modal-content">
        <div class="modal-header">
          <h3>Change Password</h3>
          <span class="close">&times;</span>
        </div>
        <form id="changePasswordForm">
          <div class="form-group">
            <label for="currentPassword">Current Password:</label>
            <input type="password" id="currentPassword" name="currentPassword" required>
          </div>
          <div class="form-group">
            <label for="newPassword">New Password:</label>
            <input type="password" id="newPassword" name="newPassword" required minlength="6">
          </div>
          <div class="form-group">
            <label for="confirmNewPassword">Confirm New Password:</label>
            <input type="password" id="confirmNewPassword" name="confirmNewPassword" required minlength="6">
          </div>
          <div class="modal-buttons">
            <button type="button" class="cancel-btn">Cancel</button>
            <button type="submit" class="submit-btn">Change Password</button>
          </div>
        </form>
        <div id="modalStatus" class="status"></div>
      </div>
    `;

    document.body.appendChild(modal);

    // Add event listeners
    modal
      .querySelector(".close")
      .addEventListener("click", () => modal.remove());
    modal
      .querySelector(".cancel-btn")
      .addEventListener("click", () => modal.remove());
    modal
      .querySelector("#changePasswordForm")
      .addEventListener("submit", handleChangePassword);

    // Click outside to close
    modal.addEventListener("click", (e) => {
      if (e.target === modal) modal.remove();
    });
  }

  // Handle password change
  async function handleChangePassword(e) {
    e.preventDefault();

    const formData = new FormData(e.target);
    const currentPassword = formData.get("currentPassword");
    const newPassword = formData.get("newPassword");
    const confirmNewPassword = formData.get("confirmNewPassword");

    if (newPassword !== confirmNewPassword) {
      showModalStatus("Passwords do not match", "error");
      return;
    }

    const submitBtn = e.target.querySelector(".submit-btn");
    const originalText = submitBtn.textContent;
    submitBtn.textContent = "Changing...";
    submitBtn.disabled = true;

    try {
      const response = await fetch("/api/auth/change-password", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          current_password: currentPassword,
          new_password: newPassword,
        }),
        credentials: "same-origin",
      });

      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(errorText || `HTTP error! status: ${response.status}`);
      }

      showModalStatus("Password changed successfully!", "success");

      setTimeout(() => {
        document.querySelector(".modal").remove();
      }, 1500);
    } catch (error) {
      console.error("Change password error:", error);
      showModalStatus(error.message || "Failed to change password", "error");
    } finally {
      submitBtn.textContent = originalText;
      submitBtn.disabled = false;
    }
  }

  // Show status in modal
  function showModalStatus(message, type) {
    const modalStatus = document.getElementById("modalStatus");
    modalStatus.textContent = message;
    modalStatus.className = `status ${type}`;
  }

  // Toggle between Spoolman and manual fields
  spoolmanEnabled.addEventListener("click", () => {
    const selectionDivider = document.querySelector(".selection-divider");

    if (spoolmanFields.style.display === "none") {
      spoolmanFields.style.display = "block";
      manualEntrySection.style.display = "none";
      if (selectionDivider) {
        selectionDivider.style.display = "none";
      }
      loadSpoolmanData();
    } else {
      spoolmanFields.style.display = "none";
      manualEntrySection.style.display = "block";
      if (selectionDivider) {
        selectionDivider.style.display = "block";
      }
    }
  });

  // Load Spoolman data
  async function loadSpoolmanData() {
    try {
      // Load spools
      const spoolsResponse = await fetch("/api/spoolman/spools");
      if (!spoolsResponse.ok) {
        if (spoolsResponse.status === 404) {
          statusDiv.textContent = "Spoolman integration is not enabled";
          statusDiv.className = "status error";
          spoolmanEnabled.checked = false;
          spoolmanFields.style.display = "none";
          manualFields.style.display = "block";
          return;
        }
        throw new Error(`Failed to load spools: ${spoolsResponse.status}`);
      }
      const spools = await spoolsResponse.json();

      // Store spools globally for filtering
      window.allSpools = spools;

      // Load materials
      const materialsResponse = await fetch("/api/spoolman/materials");
      if (!materialsResponse.ok) {
        throw new Error(
          `Failed to load materials: ${materialsResponse.status}`,
        );
      }
      const materials = await materialsResponse.json();

      // Clear and populate material select
      spoolmanMaterial.innerHTML =
        '<option value="">Select a material...</option>';
      materials.forEach((material) => {
        const option = document.createElement("option");
        option.value = material;
        option.textContent = material;
        spoolmanMaterial.appendChild(option);
      });

      // Initial population of spools
      updateSpoolList();
    } catch (error) {
      statusDiv.textContent = `Error loading Spoolman data: ${error.message}`;
      statusDiv.className = "status error";
      spoolmanEnabled.checked = false;
      spoolmanFields.style.display = "none";
      manualFields.style.display = "block";
    }
  }

  // Function to update spool list based on selected material
  function updateSpoolList() {
    const selectedMaterial = spoolmanMaterial.value;
    const filteredSpools = selectedMaterial
      ? window.allSpools.filter(
          (spool) => spool.filament.material === selectedMaterial,
        )
      : window.allSpools;

    // Clear and populate spool select
    spoolmanSpool.innerHTML = '<option value="">Select a spool...</option>';
    filteredSpools.forEach((spool) => {
      const option = document.createElement("option");
      option.value = spool.id;
      option.textContent = `${spool.filament.name} (${spool.filament.material}) - ${spool.remaining_weight}g remaining`;
      spoolmanSpool.appendChild(option);
    });
  }

  // Add material change event listener
  spoolmanMaterial.addEventListener("change", () => {
    updateSpoolList();
    // Clear spool selection when material changes
    spoolmanSpool.value = "";
    document.getElementById("colorPreview").style.backgroundColor = "";
  });

  // Handle spool selection
  spoolmanSpool.addEventListener("change", async () => {
    const spoolId = spoolmanSpool.value;
    if (!spoolId) {
      spoolmanMaterial.value = "";
      document.getElementById("colorPreview").style.backgroundColor = "";
      return;
    }

    try {
      const response = await fetch(`/api/spoolman/spool?id=${spoolId}`);
      if (!response.ok) {
        throw new Error(`Failed to load spool details: ${response.status}`);
      }
      const spool = await response.json();

      // Set the material based on the selected spool
      spoolmanMaterial.value = spool.filament.material;

      // Update color preview if color is available
      const colorPreview = document.getElementById("colorPreview");
      if (spool.filament.color_hex) {
        colorPreview.style.backgroundColor = `#${spool.filament.color_hex}`;
      } else {
        colorPreview.style.backgroundColor = "";
      }
    } catch (error) {
      statusDiv.textContent = `Error loading spool details: ${error.message}`;
      statusDiv.className = "status error";
    }
  });

  form.addEventListener("submit", async (e) => {
    e.preventDefault();

    // Get the file link value
    const fileLink = document.getElementById("fileLink").value;

    // Validate URL
    if (!isValidUrl(fileLink)) {
      statusDiv.textContent = "Please enter a valid URL for the file link";
      statusDiv.className = "status error";
      return;
    }

    const formData = {
      user_id: window.currentUser.id,
      file_link: fileLink,
      notes: document.getElementById("notes").value,
      status: "StatusPendingApproval",
    };

    // Add Spoolman or manual fields based on selection
    if (spoolmanFields.style.display === "block") {
      const spoolId = spoolmanSpool.value;
      if (spoolId) {
        formData.spool_id = parseInt(spoolId, 10);
      }
      const material = spoolmanMaterial.value;
      if (material) {
        formData.material = material;
      }
    } else {
      const color = document.getElementById("color").value;
      if (color) {
        formData.color = color;
      }
      const material = document.getElementById("material").value;
      if (material) {
        formData.material = material;
      }
    }

    try {
      const response = await fetch("/api/print-requests", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(formData),
      });

      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }

      const result = await response.json();

      // Show success message
      statusDiv.textContent = "Print job submitted successfully!";
      statusDiv.className = "status success";

      // Reset form
      form.reset();
      spoolmanFields.style.display = "none";
      manualFields.style.display = "block";
    } catch (error) {
      // Show error message
      statusDiv.textContent = `Error submitting print job: ${error.message}`;
      statusDiv.className = "status error";
    }
  });
});
