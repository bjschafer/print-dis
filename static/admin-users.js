class UserManagement {
  constructor() {
    this.users = [];
    this.currentUser = null;
    this.sortColumn = "created";
    this.sortDirection = "desc";
    this.init();
  }

  async init() {
    await this.checkAuth();
    await this.loadUsers();
    await this.loadStats();
    this.setupEventListeners();
  }

  async checkAuth() {
    // Use shared auth module
    const user = await window.authModule.checkAuthenticationStatus();
    if (!user) {
      window.location.href = "/auth.html";
      return;
    }
    
    this.currentUser = user;

    // Check if user has admin/moderator permissions
    if (!window.authModule.hasRole('moderator')) {
      alert("Access denied. Admin or moderator role required.");
      window.location.href = "/";
      return;
    }
  }

  async loadUsers() {
    try {
      const response = await fetch("/api/admin/users");
      if (!response.ok) {
        throw new Error("Failed to load users");
      }
      const responseData = await response.json();
      this.users = responseData.data || responseData;
      this.renderUsers();
    } catch (error) {
      console.error("Failed to load users:", error);
      this.showError("Failed to load users");
    }
  }

  async loadStats() {
    try {
      const response = await fetch("/api/admin/stats");
      if (!response.ok) {
        throw new Error("Failed to load stats");
      }
      const responseData = await response.json();
      const stats = responseData.data || responseData;
      this.renderStats(stats);
    } catch (error) {
      console.error("Failed to load stats:", error);
    }
  }

  renderStats(stats) {
    const statsContainer = document.getElementById("userStats");
    statsContainer.innerHTML = `
      <div class="stat-item">
        <span class="stat-label">Total Users:</span>
        <span class="stat-value">${stats.total_users}</span>
      </div>
      <div class="stat-item">
        <span class="stat-label">Active:</span>
        <span class="stat-value">${stats.enabled_users}</span>
      </div>
      <div class="stat-item">
        <span class="stat-label">Disabled:</span>
        <span class="stat-value">${stats.disabled_users}</span>
      </div>
      <div class="stat-item">
        <span class="stat-label">Admins:</span>
        <span class="stat-value">${stats.roles.admin || 0}</span>
      </div>
      <div class="stat-item">
        <span class="stat-label">Moderators:</span>
        <span class="stat-value">${stats.roles.moderator || 0}</span>
      </div>
    `;
  }

  renderUsers() {
    const tbody = document.getElementById("usersTableBody");
    tbody.innerHTML = "";

    // Sort users
    const sortedUsers = [...this.users].sort((a, b) => {
      let aVal = a[this.sortColumn];
      let bVal = b[this.sortColumn];

      if (this.sortColumn === "created") {
        aVal = new Date(aVal);
        bVal = new Date(bVal);
      }

      if (aVal < bVal) return this.sortDirection === "asc" ? -1 : 1;
      if (aVal > bVal) return this.sortDirection === "asc" ? 1 : -1;
      return 0;
    });

    sortedUsers.forEach((user) => {
      const row = document.createElement("tr");
      row.innerHTML = `
        <td>${user.username}</td>
        <td>${user.email || "N/A"}</td>
        <td>
          <span class="role-badge role-${user.role}">${user.role}</span>
        </td>
        <td>
          <span class="status-badge ${user.enabled ? "enabled" : "disabled"}">
            ${user.enabled ? "Enabled" : "Disabled"}
          </span>
        </td>
        <td>${new Date(user.created_at).toLocaleDateString()}</td>
        <td>
          <div class="action-buttons">
            ${
              this.canManageUser(user)
                ? `
              <button class="action-button update" onclick="userManagement.showRoleModal('${
                user.id
              }')">
                Change Role
              </button>
              <button class="action-button ${
                user.enabled ? "disable" : "enable"
              }" 
                      onclick="userManagement.showStatusModal('${user.id}')">
                ${user.enabled ? "Disable" : "Enable"}
              </button>
            `
                : '<span class="no-actions">No actions available</span>'
            }
          </div>
        </td>
      `;
      tbody.appendChild(row);
    });
  }

  canManageUser(user) {
    // Can't manage yourself
    if (user.id === this.currentUser.id) return false;

    // Admin can manage anyone
    if (this.currentUser.role === "admin") return true;

    // Moderator can manage regular users
    if (this.currentUser.role === "moderator" && user.role === "user")
      return true;

    return false;
  }

  showRoleModal(userId) {
    const user = this.users.find((u) => u.id === userId);
    if (!user) return;

    document.getElementById("roleUpdateUsername").textContent = user.username;
    document.getElementById("newRoleSelect").value = user.role;

    // Disable admin option for non-admins
    const adminOption = document.querySelector(
      '#newRoleSelect option[value="admin"]',
    );
    if (this.currentUser.role !== "admin") {
      adminOption.disabled = true;
    } else {
      adminOption.disabled = false;
    }

    const modal = document.getElementById("roleUpdateModal");
    modal.style.display = "block";
    modal.dataset.userId = userId;
  }

  showStatusModal(userId) {
    const user = this.users.find((u) => u.id === userId);
    if (!user) return;

    document.getElementById("statusUpdateUsername").textContent = user.username;
    document.getElementById("newStatusSelect").value = user.enabled.toString();

    const modal = document.getElementById("statusUpdateModal");
    modal.style.display = "block";
    modal.dataset.userId = userId;
  }

  async updateUserRole(userId, newRole) {
    try {
      const response = await fetch(`/api/admin/users/role?id=${userId}`, {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ role: newRole }),
      });

      if (!response.ok) {
        const error = await response.text();
        throw new Error(error);
      }

      await this.loadUsers();
      await this.loadStats();
      this.showSuccess("User role updated successfully");
    } catch (error) {
      console.error("Failed to update user role:", error);
      this.showError("Failed to update user role: " + error.message);
    }
  }

  async updateUserStatus(userId, enabled) {
    try {
      const response = await fetch(`/api/admin/users/status?id=${userId}`, {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ enabled }),
      });

      if (!response.ok) {
        const error = await response.text();
        throw new Error(error);
      }

      await this.loadUsers();
      await this.loadStats();
      this.showSuccess("User status updated successfully");
    } catch (error) {
      console.error("Failed to update user status:", error);
      this.showError("Failed to update user status: " + error.message);
    }
  }

  setupEventListeners() {
    // Sort functionality
    document.querySelectorAll(".sortable").forEach((header) => {
      header.addEventListener("click", () => {
        const column = header.dataset.sort;
        if (this.sortColumn === column) {
          this.sortDirection = this.sortDirection === "asc" ? "desc" : "asc";
        } else {
          this.sortColumn = column;
          this.sortDirection = "asc";
        }
        this.renderUsers();
      });
    });

    // Role update modal
    document
      .getElementById("confirmRoleUpdate")
      .addEventListener("click", () => {
        const modal = document.getElementById("roleUpdateModal");
        const userId = modal.dataset.userId;
        const newRole = document.getElementById("newRoleSelect").value;

        this.updateUserRole(userId, newRole);
        modal.style.display = "none";
      });

    document
      .getElementById("cancelRoleUpdate")
      .addEventListener("click", () => {
        document.getElementById("roleUpdateModal").style.display = "none";
      });

    // Status update modal
    document
      .getElementById("confirmStatusUpdate")
      .addEventListener("click", () => {
        const modal = document.getElementById("statusUpdateModal");
        const userId = modal.dataset.userId;
        const enabled =
          document.getElementById("newStatusSelect").value === "true";

        this.updateUserStatus(userId, enabled);
        modal.style.display = "none";
      });

    document
      .getElementById("cancelStatusUpdate")
      .addEventListener("click", () => {
        document.getElementById("statusUpdateModal").style.display = "none";
      });

    // Close modals when clicking outside
    window.addEventListener("click", (event) => {
      if (event.target.classList.contains("modal")) {
        event.target.style.display = "none";
      }
    });
  }

  showSuccess(message) {
    // Simple success notification
    const notification = document.createElement("div");
    notification.className = "notification success";
    notification.textContent = message;
    document.body.appendChild(notification);

    setTimeout(() => {
      notification.remove();
    }, 3000);
  }

  showError(message) {
    // Simple error notification
    const notification = document.createElement("div");
    notification.className = "notification error";
    notification.textContent = message;
    document.body.appendChild(notification);

    setTimeout(() => {
      notification.remove();
    }, 5000);
  }
}

// Initialize when page loads
const userManagement = new UserManagement();
