// Dashboard JavaScript
document.addEventListener("DOMContentLoaded", () => {
  // Global state
  let currentUser = null;
  let allRequests = [];
  let filteredRequests = [];
  let currentPage = 1;
  const itemsPerPage = 10;

  // Initialize dashboard
  init();

  async function init() {
    try {
      await checkAuthenticationStatus();
      await loadUserData();
      setupEventListeners();
      await loadPrintRequests();
    } catch (error) {
      console.error("Failed to initialize dashboard:", error);
      window.location.href = "/auth.html";
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
        throw new Error("Not authenticated");
      }

      currentUser = await response.json();
      updateUserInterface();
    } catch (error) {
      console.error("Auth check failed:", error);
      throw error;
    }
  }

  // Update user interface with user info
  function updateUserInterface() {
    const usernameElement = document.getElementById("username");
    if (usernameElement && currentUser) {
      usernameElement.textContent = `Welcome, ${currentUser.username}`;
    }

    // Show admin link if user has permissions
    const adminLink = document.getElementById("adminLink");
    if (
      adminLink &&
      currentUser &&
      (currentUser.role === "admin" || currentUser.role === "moderator")
    ) {
      adminLink.style.display = "block";
      adminLink.href = "/admin.html";
    }
  }

  // Load user-specific data
  async function loadUserData() {
    // Could load additional user-specific data here
    console.log("User data loaded for:", currentUser.username);
  }

  // Setup event listeners
  function setupEventListeners() {
    // Navigation
    document.getElementById("newRequestBtn").addEventListener("click", () => {
      window.location.href = "/index.html";
    });

    // User menu
    document
      .getElementById("logoutBtn")
      .addEventListener("click", handleLogout);
    document
      .getElementById("changePasswordBtn")
      .addEventListener("click", showChangePasswordModal);

    // Filters
    document
      .getElementById("searchInput")
      .addEventListener("input", handleSearch);
    document
      .getElementById("statusFilter")
      .addEventListener("change", handleStatusFilter);
    document.getElementById("sortBy").addEventListener("change", handleSort);
    document
      .getElementById("clearFilters")
      .addEventListener("click", clearFilters);

    // Modal
    document.getElementById("modalClose").addEventListener("click", closeModal);
    document
      .getElementById("modalCloseBtn")
      .addEventListener("click", closeModal);
    document.getElementById("requestModal").addEventListener("click", (e) => {
      if (e.target.id === "requestModal") closeModal();
    });

    // Pagination
    document
      .getElementById("prevPage")
      .addEventListener("click", () => changePage(-1));
    document
      .getElementById("nextPage")
      .addEventListener("click", () => changePage(1));
  }

  // Load print requests for current user
  async function loadPrintRequests() {
    showLoading(true);

    try {
      const response = await fetch("/api/user/print-requests", {
        method: "GET",
        credentials: "same-origin",
      });

      if (!response.ok) {
        throw new Error(`Failed to load print requests: ${response.status}`);
      }

      allRequests = await response.json();
      filteredRequests = [...allRequests];

      updateStats();
      renderRequests();
      updatePagination();

      showLoading(false);

      if (allRequests.length === 0) {
        showEmptyState();
      }
    } catch (error) {
      console.error("Failed to load print requests:", error);
      showError("Failed to load your print requests. Please try again.");
      showLoading(false);
    }
  }

  // Update statistics cards
  function updateStats() {
    const stats = {
      total: allRequests.length,
      pending: allRequests.filter(
        (r) => r.status === "StatusPendingApproval" || r.status === 0,
      ).length,
      inProgress: allRequests.filter(
        (r) =>
          r.status === "StatusEnqueued" ||
          r.status === "StatusInProgress" ||
          r.status === 1 ||
          r.status === 2,
      ).length,
      completed: allRequests.filter(
        (r) => r.status === "StatusDone" || r.status === 3,
      ).length,
    };

    document.getElementById("totalRequests").textContent = stats.total;
    document.getElementById("pendingRequests").textContent = stats.pending;
    document.getElementById("inProgressRequests").textContent =
      stats.inProgress;
    document.getElementById("completedRequests").textContent = stats.completed;
  }

  // Render print requests table
  function renderRequests() {
    const tbody = document.getElementById("requestsTableBody");
    tbody.innerHTML = "";

    const startIndex = (currentPage - 1) * itemsPerPage;
    const endIndex = startIndex + itemsPerPage;
    const pageRequests = filteredRequests.slice(startIndex, endIndex);

    pageRequests.forEach((request) => {
      const row = createRequestRow(request);
      tbody.appendChild(row);
    });

    // Show/hide table based on content
    const table = document.getElementById("requestsTable");
    if (filteredRequests.length > 0) {
      table.style.display = "block";
      document.getElementById("emptyState").style.display = "none";
    } else {
      table.style.display = "none";
      if (allRequests.length > 0) {
        showNoResultsState();
      } else {
        showEmptyState();
      }
    }
  }

  // Create a table row for a print request
  function createRequestRow(request) {
    const row = document.createElement("tr");

    // Truncate ID for display
    const shortId = request.id.substring(0, 8) + "...";

    // Format dates
    const createdDate = new Date(request.created_at).toLocaleDateString();
    const updatedDate = new Date(request.updated_at).toLocaleDateString();

    // Get file name from URL
    const fileName = getFileNameFromUrl(request.file_link);

    row.innerHTML = `
      <td>
        <code class="request-id" title="${request.id}">${shortId}</code>
      </td>
      <td>
        <a href="${
          request.file_link
        }" target="_blank" class="file-link" title="${request.file_link}">
          ${fileName}
        </a>
      </td>
      <td>
        <span class="status-badge ${getStatusClass(request.status)}">
          ${getStatusText(request.status)}
        </span>
      </td>
      <td>${request.material || "—"}</td>
      <td>${request.color || "—"}</td>
      <td>${createdDate}</td>
      <td>${updatedDate}</td>
      <td>
        <button class="action-btn primary" onclick="showRequestDetails('${
          request.id
        }')">
          View
        </button>
      </td>
    `;

    return row;
  }

  // Get file name from URL
  function getFileNameFromUrl(url) {
    try {
      const urlObj = new URL(url);
      const pathname = urlObj.pathname;
      const fileName = pathname.split("/").pop();
      return fileName || "File";
    } catch {
      return "File";
    }
  }

  // Get status CSS class
  function getStatusClass(status) {
    switch (status) {
      case "StatusPendingApproval":
      case 0:
        return "status-pending";
      case "StatusEnqueued":
      case 1:
        return "status-enqueued";
      case "StatusInProgress":
      case 2:
        return "status-in-progress";
      case "StatusDone":
      case 3:
        return "status-done";
      default:
        return "status-pending";
    }
  }

  // Get status text
  function getStatusText(status) {
    switch (status) {
      case "StatusPendingApproval":
      case 0:
        return "Pending Approval";
      case "StatusEnqueued":
      case 1:
        return "Enqueued";
      case "StatusInProgress":
      case 2:
        return "In Progress";
      case "StatusDone":
      case 3:
        return "Done";
      default:
        return "Unknown";
    }
  }

  // Show request details in modal
  window.showRequestDetails = function (requestId) {
    const request = allRequests.find((r) => r.id === requestId);
    if (!request) return;

    const modalTitle = document.getElementById("modalTitle");
    const modalBody = document.getElementById("modalBody");

    modalTitle.textContent = `Request Details`;

    modalBody.innerHTML = `
      <div class="detail-field">
        <span class="detail-label">Request ID:</span>
        <div class="detail-value"><code>${request.id}</code></div>
      </div>
      <div class="detail-field">
        <span class="detail-label">Status:</span>
        <div class="detail-value">
          <span class="status-badge ${getStatusClass(request.status)}">
            ${getStatusText(request.status)}
          </span>
        </div>
      </div>
      <div class="detail-field">
        <span class="detail-label">File Link:</span>
        <div class="detail-value">
          <a href="${request.file_link}" target="_blank">${
      request.file_link
    }</a>
        </div>
      </div>
      <div class="detail-field">
        <span class="detail-label">Material:</span>
        <div class="detail-value">${request.material || "Not specified"}</div>
      </div>
      <div class="detail-field">
        <span class="detail-label">Color:</span>
        <div class="detail-value">${request.color || "Not specified"}</div>
      </div>
      <div class="detail-field">
        <span class="detail-label">Spool ID:</span>
        <div class="detail-value">${request.spool_id || "Not specified"}</div>
      </div>
      <div class="detail-field">
        <span class="detail-label">Notes:</span>
        <div class="detail-value">${request.notes || "No notes provided"}</div>
      </div>
      <div class="detail-field">
        <span class="detail-label">Created:</span>
        <div class="detail-value">${new Date(
          request.created_at,
        ).toLocaleString()}</div>
      </div>
      <div class="detail-field">
        <span class="detail-label">Last Updated:</span>
        <div class="detail-value">${new Date(
          request.updated_at,
        ).toLocaleString()}</div>
      </div>
    `;

    document.getElementById("requestModal").style.display = "block";
  };

  // Close modal
  function closeModal() {
    document.getElementById("requestModal").style.display = "none";
  }

  // Handle search
  function handleSearch() {
    const query = document.getElementById("searchInput").value.toLowerCase();
    applyFilters();
  }

  // Handle status filter
  function handleStatusFilter() {
    applyFilters();
  }

  // Apply all filters
  function applyFilters() {
    const searchQuery = document
      .getElementById("searchInput")
      .value.toLowerCase();
    const statusFilter = document.getElementById("statusFilter").value;

    filteredRequests = allRequests.filter((request) => {
      // Search filter
      const matchesSearch =
        !searchQuery ||
        request.id.toLowerCase().includes(searchQuery) ||
        request.file_link.toLowerCase().includes(searchQuery) ||
        (request.notes && request.notes.toLowerCase().includes(searchQuery)) ||
        (request.material &&
          request.material.toLowerCase().includes(searchQuery)) ||
        (request.color && request.color.toLowerCase().includes(searchQuery));

      // Status filter
      const matchesStatus =
        !statusFilter ||
        request.status === statusFilter ||
        request.status.toString() === statusFilter;

      return matchesSearch && matchesStatus;
    });

    handleSort();
    currentPage = 1;
    renderRequests();
    updatePagination();
  }

  // Handle sorting
  function handleSort() {
    const sortBy = document.getElementById("sortBy").value;

    filteredRequests.sort((a, b) => {
      switch (sortBy) {
        case "created_at_desc":
          return new Date(b.created_at) - new Date(a.created_at);
        case "created_at_asc":
          return new Date(a.created_at) - new Date(b.created_at);
        case "updated_at_desc":
          return new Date(b.updated_at) - new Date(a.updated_at);
        case "status_asc":
          return a.status - b.status;
        default:
          return new Date(b.created_at) - new Date(a.created_at);
      }
    });

    renderRequests();
  }

  // Clear all filters
  function clearFilters() {
    document.getElementById("searchInput").value = "";
    document.getElementById("statusFilter").value = "";
    document.getElementById("sortBy").value = "created_at_desc";

    filteredRequests = [...allRequests];
    handleSort();
    currentPage = 1;
    renderRequests();
    updatePagination();
  }

  // Change page
  function changePage(direction) {
    const totalPages = Math.ceil(filteredRequests.length / itemsPerPage);
    const newPage = currentPage + direction;

    if (newPage >= 1 && newPage <= totalPages) {
      currentPage = newPage;
      renderRequests();
      updatePagination();
    }
  }

  // Update pagination controls
  function updatePagination() {
    const totalPages = Math.ceil(filteredRequests.length / itemsPerPage);
    const pagination = document.getElementById("pagination");

    if (totalPages <= 1) {
      pagination.style.display = "none";
      return;
    }

    pagination.style.display = "flex";

    document.getElementById("prevPage").disabled = currentPage === 1;
    document.getElementById("nextPage").disabled = currentPage === totalPages;
    document.getElementById(
      "pageInfo",
    ).textContent = `Page ${currentPage} of ${totalPages}`;
  }

  // Show loading state
  function showLoading(show) {
    const spinner = document.getElementById("loadingSpinner");
    const table = document.getElementById("requestsTable");
    const emptyState = document.getElementById("emptyState");

    if (show) {
      spinner.style.display = "flex";
      table.style.display = "none";
      emptyState.style.display = "none";
    } else {
      spinner.style.display = "none";
    }
  }

  // Show empty state
  function showEmptyState() {
    const emptyState = document.getElementById("emptyState");
    emptyState.innerHTML = `
      <div class="empty-icon">📄</div>
      <h3>No print requests found</h3>
      <p>You haven't submitted any print requests yet.</p>
      <button class="btn btn-primary" onclick="window.location.href='index.html'">
        Submit Your First Request
      </button>
    `;
    emptyState.style.display = "block";
  }

  // Show no results state (when filters return empty)
  function showNoResultsState() {
    const emptyState = document.getElementById("emptyState");
    emptyState.innerHTML = `
      <div class="empty-icon">🔍</div>
      <h3>No matching requests found</h3>
      <p>Try adjusting your search or filter criteria.</p>
      <button class="btn btn-secondary" onclick="clearFilters()">
        Clear Filters
      </button>
    `;
    emptyState.style.display = "block";
  }

  // Show error message
  function showError(message) {
    // You could implement a toast notification system here
    alert(message);
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

    // Reuse the modal functionality from app.js or implement a simpler version
    alert("Change password functionality will be implemented soon!");
  }

  // Make clearFilters available globally for the no results state button
  window.clearFilters = clearFilters;
});
