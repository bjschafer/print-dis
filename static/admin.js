// Global function to display print requests
function displayPrintRequests(printRequests) {
  const printRequestsTableBody = document.getElementById(
    "printRequestsTableBody",
  );
  printRequestsTableBody.innerHTML = "";

  printRequests.forEach((request) => {
    const row = document.createElement("tr");

    // Create a link to the file
    const fileLink = document.createElement("a");
    fileLink.href = request.file_link;
    fileLink.textContent = request.file_link.split("/").pop() || "View File";
    fileLink.target = "_blank";
    fileLink.className = "file-link";

    row.innerHTML = `
      <td title="${request.id}">${request.id}</td>
      <td>${request.user_id}</td>
      <td></td>
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
              <button class="action-button update" onclick="updateStatus('${
                request.id
              }', '${request.status}')">
                  Update Status
              </button>
              <button class="action-button delete" onclick="deleteRequest('${
                request.id
              }')">
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
    const status = document.getElementById("statusFilter").value;
    const url = status
      ? `/api/print-requests?status=${status}`
      : "/api/print-requests";

    const response = await fetch(url, {
      headers: {
        Accept: "application/json",
      },
    });

    if (!response.ok) {
      throw new Error("Failed to fetch print requests");
    }

    const printRequests = await response.json();
    displayPrintRequests(printRequests);
  } catch (error) {
    console.error("Error loading print requests:", error);
    alert("Failed to load print requests. Please try again.");
  }
}

document.addEventListener("DOMContentLoaded", () => {
  const statusFilter = document.getElementById("statusFilter");

  // Load print requests on page load
  loadPrintRequests();

  // Add event listener for status filter
  statusFilter.addEventListener("change", loadPrintRequests);

  // Add click handler for copying IDs
  document.addEventListener("click", (e) => {
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
});

let currentRequestId = null;
let currentStatus = null;

function showStatusUpdateModal(requestId, currentStatus) {
  const modal = document.getElementById("statusUpdateModal");
  const select = document.getElementById("newStatusSelect");

  // Define all possible statuses
  const allStatuses = [
    "StatusPendingApproval",
    "StatusEnqueued",
    "StatusInProgress",
    "StatusDone",
  ];

  // Clear and populate the select options
  select.innerHTML = "";
  allStatuses.forEach((status) => {
    const option = document.createElement("option");
    option.value = status;
    option.textContent = status.replace("Status", "");
    select.appendChild(option);
  });

  // Store the current request ID and status
  currentRequestId = requestId;
  currentStatus = currentStatus;

  // Show the modal
  modal.style.display = "block";
}

async function updateStatus(requestId, currentStatus) {
  showStatusUpdateModal(requestId, currentStatus);
}

// Add event listeners for the modal buttons
document.addEventListener("DOMContentLoaded", () => {
  const statusFilter = document.getElementById("statusFilter");
  const modal = document.getElementById("statusUpdateModal");
  const confirmButton = document.getElementById("confirmStatusUpdate");
  const cancelButton = document.getElementById("cancelStatusUpdate");

  // Load print requests on page load
  loadPrintRequests();

  // Add event listener for status filter
  statusFilter.addEventListener("change", loadPrintRequests);

  // Add click handler for copying IDs
  document.addEventListener("click", (e) => {
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
        throw new Error("Failed to update status");
      }

      // Hide the modal
      modal.style.display = "none";

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
  });

  // Close modal when clicking outside
  window.addEventListener("click", (event) => {
    if (event.target === modal) {
      modal.style.display = "none";
    }
  });
});

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
