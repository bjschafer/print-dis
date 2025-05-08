document.addEventListener("DOMContentLoaded", () => {
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

  // URL validation function
  function isValidUrl(url) {
    try {
      new URL(url);
      return true;
    } catch (e) {
      return false;
    }
  }

  // Toggle between Spoolman and manual fields
  spoolmanEnabled.addEventListener("click", () => {
    if (spoolmanFields.style.display === "none") {
      spoolmanFields.style.display = "block";
      manualEntrySection.style.display = "none";
      loadSpoolmanData();
    } else {
      spoolmanFields.style.display = "none";
      manualEntrySection.style.display = "block";
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
      user_id: document.getElementById("submitter").value,
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
