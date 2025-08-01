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
    // Use shared auth module for authentication
    const user = await window.authModule.checkAuthenticationStatus();
    if (!user) {
      // User is not authenticated, redirect to auth page
      window.location.href = "/auth.html";
      return;
    }

    // Hide the submitter field since we now know the user
    const submitterField = document.getElementById("submitter");
    if (submitterField) {
      submitterField.closest(".form-group").style.display = "none";
      // Remove the required attribute to prevent form validation issues
      submitterField.removeAttribute("required");
      // Set a default value since we know the user
      submitterField.value = user.username;
    }

    // Store user info globally for compatibility (keep existing code working)
    window.currentUser = user;
    
    // Check if spoolman is available
    await checkSpoolmanAvailability();
  }

  // Check if spoolman is available and hide button if not
  async function checkSpoolmanAvailability() {
    try {
      const response = await fetch("/api/spoolman/spools");
      if (!response.ok) {
        // Spoolman is not available, hide the button
        hideSpoolmanButton();
      }
    } catch (error) {
      // Network error or spoolman not available
      hideSpoolmanButton();
    }
  }

  // Hide the spoolman button and show manual only
  function hideSpoolmanButton() {
    const spoolmanSection = spoolmanEnabled.closest(".selection-option");
    const selectionDivider = document.querySelector(".selection-divider");
    
    if (spoolmanSection) {
      spoolmanSection.style.display = "none";
    }
    if (selectionDivider) {
      selectionDivider.style.display = "none";
    }
    
    // Update the manual section title to remove "OR" implication
    const manualTitle = document.querySelector(".selection-option h4");
    if (manualTitle) {
      manualTitle.textContent = "Filament Information";
    }
  }


  // Set up event listeners for the existing HTML user menu elements
  function setupUserMenuEventListeners() {
    // Add event listeners for the existing HTML elements
    const logoutBtn = document.getElementById("logoutBtn");
    const changePasswordBtn = document.getElementById("changePasswordBtn");

    if (logoutBtn) {
      logoutBtn.addEventListener("click", function(e) {
        e.preventDefault();
        window.authModule.handleLogout();
      });
    }

    if (changePasswordBtn) {
      changePasswordBtn.addEventListener("click", function(e) {
        e.preventDefault();
        window.authModule.showChangePasswordModal();
      });
    }
  }

  // Authentication functions are now handled by shared-auth.js module

  // Password change functionality is now handled by shared-auth.js module

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
          spoolmanFields.style.display = "none";
          manualEntrySection.style.display = "block";
          const selectionDivider = document.querySelector(".selection-divider");
          if (selectionDivider) {
            selectionDivider.style.display = "block";
          }
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
      spoolmanFields.style.display = "none";
      manualEntrySection.style.display = "block";
      const selectionDivider = document.querySelector(".selection-divider");
      if (selectionDivider) {
        selectionDivider.style.display = "block";
      }
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
