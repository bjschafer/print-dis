// Accessibility utilities and enhancements
window.AccessibilityModule = (function() {
  'use strict';

  // Focus management
  let focusStack = [];
  let currentFocusedElement = null;

  // Keyboard navigation settings
  const KEYBOARD_NAVIGATION = {
    ESCAPE: 'Escape',
    TAB: 'Tab',
    ENTER: 'Enter',
    SPACE: ' ',
    ARROW_UP: 'ArrowUp',
    ARROW_DOWN: 'ArrowDown',
    ARROW_LEFT: 'ArrowLeft',
    ARROW_RIGHT: 'ArrowRight',
    HOME: 'Home',
    END: 'End'
  };

  // Live regions for screen reader announcements
  let liveRegions = {
    polite: null,
    assertive: null
  };

  // Initialize accessibility features
  function init() {
    createLiveRegions();
    setupSkipLinks();
    setupKeyboardNavigation();
    setupFocusManagement();
    setupModalAccessibility();
    setupTableAccessibility();
    setupFormAccessibility();
    addARIALabels();
  }

  // Create live regions for screen reader announcements
  function createLiveRegions() {
    // Polite live region for non-urgent announcements
    liveRegions.polite = document.createElement('div');
    liveRegions.polite.setAttribute('aria-live', 'polite');
    liveRegions.polite.setAttribute('aria-atomic', 'true');
    liveRegions.polite.setAttribute('class', 'sr-only');
    liveRegions.polite.id = 'live-region-polite';
    document.body.appendChild(liveRegions.polite);

    // Assertive live region for urgent announcements
    liveRegions.assertive = document.createElement('div');
    liveRegions.assertive.setAttribute('aria-live', 'assertive');
    liveRegions.assertive.setAttribute('aria-atomic', 'true');
    liveRegions.assertive.setAttribute('class', 'sr-only');
    liveRegions.assertive.id = 'live-region-assertive';
    document.body.appendChild(liveRegions.assertive);
  }

  // Setup skip links for keyboard navigation
  function setupSkipLinks() {
    const skipLinks = document.createElement('div');
    skipLinks.className = 'skip-links';
    skipLinks.innerHTML = `
      <a href="#main-content" class="skip-link">Skip to main content</a>
      <a href="#main-nav" class="skip-link">Skip to navigation</a>
    `;
    document.body.insertBefore(skipLinks, document.body.firstChild);
  }

  // Setup keyboard navigation
  function setupKeyboardNavigation() {
    document.addEventListener('keydown', handleGlobalKeyDown);
    
    // Setup roving tabindex for button groups
    setupRovingTabIndex();
  }

  // Global keyboard event handler
  function handleGlobalKeyDown(event) {
    switch (event.key) {
      case KEYBOARD_NAVIGATION.ESCAPE:
        handleEscapeKey(event);
        break;
      case KEYBOARD_NAVIGATION.TAB:
        handleTabKey(event);
        break;
    }
  }

  // Handle escape key - close modals, dropdowns, etc.
  function handleEscapeKey(event) {
    // Close any open modals
    const openModals = document.querySelectorAll('.modal[style*="display: block"], .modal:not([style*="display: none"])');
    openModals.forEach(modal => {
      if (modal.style.display !== 'none') {
        closeModal(modal);
      }
    });

    // Close any open dropdowns
    const openDropdowns = document.querySelectorAll('.dropdown-content.show');
    openDropdowns.forEach(dropdown => {
      dropdown.classList.remove('show');
    });

    // Announce escape action
    announce('Dialog closed', 'polite');
  }

  // Handle tab key for focus trapping
  function handleTabKey(event) {
    const activeElement = document.activeElement;
    
    // Check if we're in a modal
    const modal = activeElement.closest('.modal');
    if (modal && modal.style.display !== 'none') {
      trapFocus(event, modal);
    }
  }

  // Trap focus within a container
  function trapFocus(event, container) {
    const focusableElements = container.querySelectorAll(
      'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])'
    );
    
    if (focusableElements.length === 0) return;

    const firstElement = focusableElements[0];
    const lastElement = focusableElements[focusableElements.length - 1];

    if (event.shiftKey) {
      if (document.activeElement === firstElement) {
        lastElement.focus();
        event.preventDefault();
      }
    } else {
      if (document.activeElement === lastElement) {
        firstElement.focus();
        event.preventDefault();
      }
    }
  }

  // Setup roving tabindex for button groups
  function setupRovingTabIndex() {
    const buttonGroups = document.querySelectorAll('[role="group"]');
    
    buttonGroups.forEach(group => {
      const buttons = group.querySelectorAll('button');
      if (buttons.length > 1) {
        // Set first button as focusable
        buttons[0].setAttribute('tabindex', '0');
        
        // Set others as non-focusable
        for (let i = 1; i < buttons.length; i++) {
          buttons[i].setAttribute('tabindex', '-1');
        }

        // Add keyboard navigation
        group.addEventListener('keydown', (event) => {
          handleButtonGroupNavigation(event, buttons);
        });
      }
    });
  }

  // Handle keyboard navigation within button groups
  function handleButtonGroupNavigation(event, buttons) {
    const currentIndex = Array.from(buttons).indexOf(document.activeElement);
    let newIndex;

    switch (event.key) {
      case KEYBOARD_NAVIGATION.ARROW_LEFT:
      case KEYBOARD_NAVIGATION.ARROW_UP:
        newIndex = currentIndex > 0 ? currentIndex - 1 : buttons.length - 1;
        break;
      case KEYBOARD_NAVIGATION.ARROW_RIGHT:
      case KEYBOARD_NAVIGATION.ARROW_DOWN:
        newIndex = currentIndex < buttons.length - 1 ? currentIndex + 1 : 0;
        break;
      case KEYBOARD_NAVIGATION.HOME:
        newIndex = 0;
        break;
      case KEYBOARD_NAVIGATION.END:
        newIndex = buttons.length - 1;
        break;
      default:
        return;
    }

    event.preventDefault();
    buttons[currentIndex].setAttribute('tabindex', '-1');
    buttons[newIndex].setAttribute('tabindex', '0');
    buttons[newIndex].focus();
  }

  // Setup focus management
  function setupFocusManagement() {
    // Track focus changes
    document.addEventListener('focusin', (event) => {
      currentFocusedElement = event.target;
    });

    // Handle focus indicators
    document.addEventListener('keydown', (event) => {
      if (event.key === KEYBOARD_NAVIGATION.TAB) {
        document.body.classList.add('keyboard-focus');
      }
    });

    document.addEventListener('mousedown', () => {
      document.body.classList.remove('keyboard-focus');
    });
  }

  // Setup modal accessibility
  function setupModalAccessibility() {
    const modals = document.querySelectorAll('.modal');
    
    modals.forEach(modal => {
      // Add proper ARIA attributes
      modal.setAttribute('role', 'dialog');
      modal.setAttribute('aria-modal', 'true');
      modal.setAttribute('aria-hidden', 'true');
      
      // Add aria-labelledby if there's a title
      const title = modal.querySelector('h2, h3, .modal-title');
      if (title) {
        if (!title.id) {
          title.id = 'modal-title-' + Math.random().toString(36).substr(2, 9);
        }
        modal.setAttribute('aria-labelledby', title.id);
      }
    });
  }

  // Setup table accessibility
  function setupTableAccessibility() {
    const tables = document.querySelectorAll('table');
    
    tables.forEach(table => {
      // Add role and aria-label
      table.setAttribute('role', 'table');
      
      // Add caption if missing
      if (!table.querySelector('caption')) {
        const caption = document.createElement('caption');
        caption.textContent = table.getAttribute('aria-label') || 'Data table';
        caption.className = 'sr-only';
        table.insertBefore(caption, table.firstChild);
      }

      // Enhance sortable headers
      const sortableHeaders = table.querySelectorAll('.sortable');
      sortableHeaders.forEach(header => {
        header.setAttribute('role', 'columnheader');
        header.setAttribute('tabindex', '0');
        
        // Add aria-sort attribute
        if (!header.hasAttribute('aria-sort')) {
          header.setAttribute('aria-sort', 'none');
        }

        // Add keyboard support
        header.addEventListener('keydown', (event) => {
          if (event.key === KEYBOARD_NAVIGATION.ENTER || event.key === KEYBOARD_NAVIGATION.SPACE) {
            event.preventDefault();
            header.click();
          }
        });
      });
    });
  }

  // Setup form accessibility
  function setupFormAccessibility() {
    const forms = document.querySelectorAll('form');
    
    forms.forEach(form => {
      // Add proper labels and associations
      const inputs = form.querySelectorAll('input, select, textarea');
      
      inputs.forEach(input => {
        // Ensure proper labeling
        if (!input.getAttribute('aria-label') && !input.getAttribute('aria-labelledby')) {
          const label = form.querySelector(`label[for="${input.id}"]`);
          if (label) {
            // Label is properly associated
          } else {
            // Look for adjacent label
            const adjacentLabel = input.previousElementSibling || input.parentElement.querySelector('label');
            if (adjacentLabel && adjacentLabel.tagName === 'LABEL') {
              if (!adjacentLabel.getAttribute('for')) {
                adjacentLabel.setAttribute('for', input.id || generateId());
              }
            }
          }
        }

        // Add aria-required for required fields
        if (input.hasAttribute('required')) {
          input.setAttribute('aria-required', 'true');
        }

        // Add aria-invalid for validation
        input.addEventListener('blur', () => {
          if (input.validity && !input.validity.valid) {
            input.setAttribute('aria-invalid', 'true');
          } else {
            input.setAttribute('aria-invalid', 'false');
          }
        });
      });
    });
  }

  // Add ARIA labels to existing elements
  function addARIALabels() {
    // Navigation
    const nav = document.querySelector('nav');
    if (nav) {
      nav.setAttribute('role', 'navigation');
      nav.setAttribute('aria-label', 'Main navigation');
      nav.id = 'main-nav';
    }

    // Main content
    const main = document.querySelector('main');
    if (main) {
      main.id = 'main-content';
      main.setAttribute('role', 'main');
    }

    // Buttons without proper labels
    const buttons = document.querySelectorAll('button:not([aria-label]):not([aria-labelledby])');
    buttons.forEach(button => {
      if (!button.textContent.trim()) {
        // Button with no text - try to infer from context
        const icon = button.querySelector('.btn-icon');
        if (icon && icon.textContent === '+') {
          button.setAttribute('aria-label', 'Add new item');
        } else if (button.querySelector('.close, .×')) {
          button.setAttribute('aria-label', 'Close');
        }
      }
    });

    // Search inputs
    const searchInputs = document.querySelectorAll('input[type="search"], input[placeholder*="search" i]');
    searchInputs.forEach(input => {
      if (!input.getAttribute('aria-label')) {
        input.setAttribute('aria-label', 'Search');
      }
    });

    // Status indicators
    const statusElements = document.querySelectorAll('.status, .stat-value');
    statusElements.forEach(element => {
      element.setAttribute('role', 'status');
      element.setAttribute('aria-live', 'polite');
    });
  }

  // Public API functions

  // Announce message to screen readers
  function announce(message, priority = 'polite') {
    const region = liveRegions[priority];
    if (region) {
      region.textContent = message;
      
      // Clear after announcement
      setTimeout(() => {
        region.textContent = '';
      }, 1000);
    }
  }

  // Save current focus
  function saveFocus() {
    focusStack.push(document.activeElement);
  }

  // Restore previous focus
  function restoreFocus() {
    if (focusStack.length > 0) {
      const element = focusStack.pop();
      if (element && element.focus) {
        element.focus();
      }
    }
  }

  // Open modal with proper focus management
  function openModal(modal) {
    if (typeof modal === 'string') {
      modal = document.getElementById(modal);
    }
    
    if (!modal) return;

    // Save current focus
    saveFocus();

    // Show modal
    modal.style.display = 'block';
    modal.setAttribute('aria-hidden', 'false');

    // Focus first focusable element
    const firstFocusable = modal.querySelector('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])');
    if (firstFocusable) {
      firstFocusable.focus();
    }

    // Announce modal opening
    const title = modal.querySelector('h2, h3, .modal-title');
    if (title) {
      announce(`${title.textContent} dialog opened`, 'assertive');
    }
  }

  // Close modal with proper focus management
  function closeModal(modal) {
    if (typeof modal === 'string') {
      modal = document.getElementById(modal);
    }
    
    if (!modal) return;

    // Hide modal
    modal.style.display = 'none';
    modal.setAttribute('aria-hidden', 'true');

    // Restore focus
    restoreFocus();

    // Announce modal closing
    announce('Dialog closed', 'polite');
  }

  // Generate unique ID
  function generateId() {
    return 'accessibility-' + Math.random().toString(36).substr(2, 9);
  }

  // Update sort indicators
  function updateSortIndicator(header, direction) {
    const allHeaders = header.parentElement.querySelectorAll('.sortable');
    
    // Reset all headers
    allHeaders.forEach(h => {
      h.setAttribute('aria-sort', 'none');
      const indicator = h.querySelector('.sort-indicator');
      if (indicator) {
        indicator.textContent = '';
      }
    });

    // Set current header
    header.setAttribute('aria-sort', direction);
    const indicator = header.querySelector('.sort-indicator');
    if (indicator) {
      indicator.textContent = direction === 'ascending' ? '↑' : '↓';
    }

    // Announce sort change
    announce(`Table sorted by ${header.textContent} ${direction}`, 'polite');
  }

  // Public API
  return {
    init,
    announce,
    saveFocus,
    restoreFocus,
    openModal,
    closeModal,
    updateSortIndicator,
    trapFocus
  };
})();

// Initialize when DOM is ready
if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', window.AccessibilityModule.init);
} else {
  window.AccessibilityModule.init();
}