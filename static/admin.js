// Global variables for sorting
let currentSort = {
  column: "created",
  direction: "desc",
};

// Global variables for user data
let usersMap = new Map(); // Map of user_id -> user object
let spoolmanConfig = null; // Spoolman configuration

// Global variables for status updates
let currentRequestId = null;
let currentStatus = null;

// Function to load spoolman configuration
async function loadSpoolmanConfig() {
  try {
    const response = await fetch("/api/admin/spoolman-config", {
      headers: {
        Accept: "application/json",
      },
    });

    if (!response.ok) {
      throw new Error("Failed to fetch spoolman config");
    }

    const responseData = await response.json();
    spoolmanConfig = responseData.data || responseData;
    console.log("Loaded spoolman config:", spoolmanConfig);
  } catch (error) {
    console.error("Error loading spoolman config:", error);
    // Set default config if loading fails
    spoolmanConfig = { enabled: false };
  }
}

// Function to load users data
async function loadUsers() {
  try {
    const response = await fetch("/api/admin/users", {
      headers: {
        Accept: "application/json",
      },
    });

    if (!response.ok) {
      throw new Error("Failed to fetch users");
    }

    const responseData = await response.json();
    const users = responseData.data || responseData;

    // Create a map for quick lookups
    usersMap.clear();
    users.forEach((user) => {
      usersMap.set(user.id, user);
    });

    console.log("Loaded users:", users.length);
  } catch (error) {
    console.error("Error loading users:", error);
    // Don't alert for users loading error, as it's not critical
  }
}

// Global function to display print requests
function displayPrintRequests(printRequests) {
  const printRequestsTableBody = document.getElementById(
    "printRequestsTableBody",
  );
  printRequestsTableBody.innerHTML = "";

  // Sort the print requests
  printRequests.sort((a, b) => {
    let valueA, valueB;

    switch (currentSort.column) {
      case "id":
        valueA = a.id;
        valueB = b.id;
        break;
      case "user":
        // Sort by display name or username instead of user_id
        const userA = usersMap.get(a.user_id);
        const userB = usersMap.get(b.user_id);
        valueA = userA ? userA.display_name || userA.username : a.user_id;
        valueB = userB ? userB.display_name || userB.username : b.user_id;
        break;
      case "file":
        valueA = a.file_link;
        valueB = b.file_link;
        break;
      case "filament":
        // Sort by filament name or material
        if (a.spool_details && b.spool_details) {
          valueA = a.spool_details.filament.name;
          valueB = b.spool_details.filament.name;
        } else if (a.material && b.material) {
          valueA = a.material;
          valueB = b.material;
        } else {
          valueA = a.spool_details
            ? a.spool_details.filament.name
            : a.material || "";
          valueB = b.spool_details
            ? b.spool_details.filament.name
            : b.material || "";
        }
        break;
      case "notes":
        valueA = a.notes || "";
        valueB = b.notes || "";
        break;
      case "status":
        valueA = a.status;
        valueB = b.status;
        break;
      case "created":
        valueA = new Date(a.created_at).getTime();
        valueB = new Date(b.created_at).getTime();
        break;
      default:
        return 0;
    }

    if (currentSort.direction === "asc") {
      return valueA > valueB ? 1 : -1;
    } else {
      return valueA < valueB ? 1 : -1;
    }
  });

  printRequests.forEach((request) => {
    const row = document.createElement("tr");

    // Create a link to the file
    const fileLink = document.createElement("a");
    fileLink.href = request.file_link;
    fileLink.textContent = request.file_link.split("/").pop() || "View File";
    fileLink.target = "_blank";
    fileLink.className = "file-link";

    // Get user display name or fallback to user_id
    const user = usersMap.get(request.user_id);
    const userDisplay = user
      ? user.display_name || user.username
      : `User (${request.user_id.substring(0, 8)}...)`;

    // Format filament/spool information
    let filamentInfo = "Not specified";
    if (request.spool_details) {
      const spool = request.spool_details;
      const filament = spool.filament;
      let colorDisplay = "";
      if (filament.color_hex) {
        colorDisplay = `<span class="color-swatch" style="background-color: #${filament.color_hex};" title="#${filament.color_hex}"></span>`;
      }

      if (spoolmanConfig && spoolmanConfig.enabled && spoolmanConfig.base_url) {
        filamentInfo = `<a href="${spoolmanConfig.base_url}/spool/show/${spool.id}" target="_blank" class="spool-link">
          ${colorDisplay}${filament.vendor.name} ${filament.name} (${filament.material})
          <br><small>Spool #${spool.id} - ${spool.remaining_weight}g remaining</small>
        </a>`;
      } else {
        filamentInfo = `${colorDisplay}${filament.vendor.name} ${filament.name} (${filament.material})
          <br><small>Spool #${spool.id} - ${spool.remaining_weight}g remaining</small>`;
      }
    } else if (request.material || request.color) {
      const parts = [];
      if (request.material) parts.push(request.material);
      if (request.color) parts.push(`Color: ${request.color}`);
      filamentInfo = parts.join(", ");
    }

    // Format notes with truncation
    let notesDisplay = "No notes";
    if (request.notes && request.notes.trim()) {
      const maxLength = 50;
      const truncated =
        request.notes.length > maxLength
          ? request.notes.substring(0, maxLength) + "..."
          : request.notes;
      notesDisplay = `<span title="${request.notes}">${truncated}</span>`;
    }

    row.innerHTML = `
      <td title="${request.id}">${request.id}</td>
      <td title="${request.user_id}">${userDisplay}</td>
      <td></td>
      <td class="filament-cell">${filamentInfo}</td>
      <td class="notes-cell">${notesDisplay}</td>
      <td>
          <span class="status-badge status-${request.status
            .toLowerCase()
            .replace("_", "-")}">
              ${request.status.replace("Status", "")}
          </span>
      </td>
      <td>${new Date(request.created_at).toLocaleString()}</td>
      <td>
          <div class="action-buttons">
              <button class="action-button update" data-request-id="${request.id}" data-status="${request.status}">
                  Update Status
              </button>
              <button class="action-button delete" data-request-id="${request.id}">
                  Delete
              </button>
          </div>
      </td>
    `;

    // Insert the file link into the empty td
    const fileCell = row.querySelector("td:nth-child(3)");
    fileCell.appendChild(fileLink);

    printRequestsTableBody.appendChild(row);
  });
}

// Global function to load print requests
async function loadPrintRequests() {
  try {
    // Load users and spoolman config in parallel if not already loaded
    const usersPromise = usersMap.size === 0 ? loadUsers() : Promise.resolve();
    const spoolmanPromise =
      spoolmanConfig === null ? loadSpoolmanConfig() : Promise.resolve();

    const status = document.getElementById("statusFilter").value;
    const url = status
      ? `/api/admin/print-requests?status=${status}`
      : "/api/admin/print-requests";

    const response = await fetch(url, {
      headers: {
        Accept: "application/json",
      },
    });

    if (!response.ok) {
      throw new Error("Failed to fetch print requests");
    }

    const [responseData] = await Promise.all([
      response.json(),
      usersPromise,
      spoolmanPromise,
    ]);

    // Extract data from response wrapper (API returns {success, data, message})
    const printRequests = responseData.data || responseData;
    displayPrintRequests(printRequests);
  } catch (error) {
    console.error("Error loading print requests:", error);
    alert("Failed to load print requests. Please try again.");
  }
}

document.addEventListener("DOMContentLoaded", async () => {
  // Check authentication and require moderator role
  const user = await window.authModule.checkAuthenticationStatus();
  if (!user) {
    window.location.href = "/auth.html";
    return;
  }
  
  if (!window.authModule.hasRole('moderator')) {
    window.location.href = "/dashboard.html";
    return;
  }

  const statusFilter = document.getElementById("statusFilter");
  const modal = document.getElementById("statusUpdateModal");
  const confirmButton = document.getElementById("confirmStatusUpdate");
  const cancelButton = document.getElementById("cancelStatusUpdate");

  // Add sorting event listeners
  document.querySelectorAll("th.sortable").forEach((header) => {
    header.addEventListener("click", () => {
      const column = header.dataset.sort;

      // Update sort direction
      if (currentSort.column === column) {
        currentSort.direction =
          currentSort.direction === "asc" ? "desc" : "asc";
      } else {
        currentSort.column = column;
        currentSort.direction = "asc";
      }

      // Update sort indicators
      document.querySelectorAll("th.sortable").forEach((th) => {
        th.removeAttribute("data-sort-direction");
      });
      header.setAttribute("data-sort-direction", currentSort.direction);

      // Reload the print requests to apply sorting
      loadPrintRequests();
    });
  });

  // Set initial sort indicator
  document
    .querySelector(`th[data-sort="${currentSort.column}"]`)
    .setAttribute("data-sort-direction", currentSort.direction);

  // Load print requests on page load
  loadPrintRequests();

  // Add event listener for status filter
  statusFilter.addEventListener("change", loadPrintRequests);

  // Add event delegation for action buttons and ID copying
  document.addEventListener("click", (e) => {
    // Handle update status button
    if (e.target.classList.contains("update") && e.target.dataset.requestId) {
      const requestId = e.target.dataset.requestId;
      const currentStatus = e.target.dataset.status;
      updateStatus(requestId, currentStatus);
      return;
    }
    
    // Handle delete button
    if (e.target.classList.contains("delete") && e.target.dataset.requestId) {
      const requestId = e.target.dataset.requestId;
      deleteRequest(requestId);
      return;
    }

    // Handle ID copying
    const idCell = e.target.closest("td[title]");
    if (idCell) {
      const id = idCell.getAttribute("title");
      // Create a temporary input element
      const tempInput = document.createElement("input");
      tempInput.value = id;
      document.body.appendChild(tempInput);
      tempInput.select();

      try {
        // Try to copy using execCommand
        const successful = document.execCommand("copy");
        if (successful) {
          // Show feedback
          const originalText = idCell.textContent;
          idCell.textContent = "✓ Copied!";
          idCell.style.color = "#28a745";
          setTimeout(() => {
            idCell.textContent = originalText;
            idCell.style.color = "#6c757d";
          }, 1000);
        }
      } catch (err) {
        console.error("Failed to copy ID:", err);
      } finally {
        // Clean up
        document.body.removeChild(tempInput);
      }
    }
  });

  // Handle confirm button click
  confirmButton.addEventListener("click", async () => {
    const select = document.getElementById("newStatusSelect");
    const newStatus = select.value;

    try {
      const response = await fetch(
        `/api/print-requests/status?id=${currentRequestId}`,
        {
          method: "PATCH",
          headers: {
            "Content-Type": "application/json",
            "Accept": "application/json",
          },
          body: JSON.stringify({
            status: newStatus,
          }),
        },
      );

      if (!response.ok) {
        const errorData = await response.json();
        const errorMessage = errorData.error?.details || errorData.error?.message || "Failed to update status";
        throw new Error(errorMessage);
      }

      // Hide the modal
      modal.style.display = "none";
      modal.setAttribute("aria-hidden", "true");

      // Reload the print requests to show the updated status
      loadPrintRequests();
    } catch (error) {
      console.error("Error updating status:", error);
      alert("Failed to update status. Please try again.");
    }
  });

  // Handle cancel button click
  cancelButton.addEventListener("click", () => {
    modal.style.display = "none";
    modal.setAttribute("aria-hidden", "true");
  });

  // Close modal when clicking outside
  window.addEventListener("click", (event) => {
    if (event.target === modal) {
      modal.style.display = "none";
      modal.setAttribute("aria-hidden", "true");
    }
  });
});

function showStatusUpdateModal(requestId, currentStatus) {
  const modal = document.getElementById("statusUpdateModal");
  const select = document.getElementById("newStatusSelect");

  // Define valid status transitions based on backend logic
  const validTransitions = {
    "StatusPendingApproval": ["StatusEnqueued", "StatusPendingApproval"],
    "StatusEnqueued": ["StatusInProgress", "StatusPendingApproval", "StatusEnqueued"],
    "StatusInProgress": ["StatusDone", "StatusEnqueued", "StatusInProgress"],
    "StatusDone": ["StatusDone"]
  };

  // Get valid statuses for the current status
  const validStatuses = validTransitions[currentStatus] || [];

  // Clear and populate the select options with only valid transitions
  select.innerHTML = "";
  validStatuses.forEach((status) => {
    const option = document.createElement("option");
    option.value = status;
    option.textContent = status.replace("Status", "");
    if (status === currentStatus) {
      option.selected = true;
    }
    select.appendChild(option);
  });

  // Store the current request ID and status
  currentRequestId = requestId;
  currentStatus = currentStatus;

  // Show the modal
  modal.style.display = "block";
  modal.setAttribute("aria-hidden", "false");
}

async function updateStatus(requestId, currentStatus) {
  showStatusUpdateModal(requestId, currentStatus);
}

async function deleteRequest(requestId) {
  if (!confirm("Are you sure you want to delete this print request?")) {
    return;
  }

  try {
    const response = await fetch(`/api/print-requests?id=${requestId}`, {
      method: "DELETE",
      headers: {
        Accept: "application/json",
      },
    });

    if (!response.ok) {
      throw new Error("Failed to delete print request");
    }

    // Reload the print requests to show the updated list
    loadPrintRequests();
  } catch (error) {
    console.error("Error deleting print request:", error);
    alert("Failed to delete print request. Please try again.");
  }
}
