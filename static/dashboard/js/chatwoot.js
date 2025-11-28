// =========================================================================
// CHATWOOT INTEGRATION FUNCTIONS
// =========================================================================

/**
 * Initialize Chatwoot configuration modal and listeners
 */
function initChatwootConfig() {
  // Initialize accordion
  $('.ui.accordion').accordion();

  // Initialize Chatwoot enabled toggle
  $('#chatwootEnabledToggle').checkbox();
  $('#chatwootAutoCreateToggle').checkbox();
  
  // All other toggles
  $('#chatwootSignMsg').checkbox();
  $('#chatwootReopenConversation').checkbox();
  $('#chatwootConversationPending').checkbox();
  $('#chatwootMergeBrazilContacts').checkbox();

  // Copy webhook URL button
  $('#copyWebhookUrl').on('click', function() {
    const webhookUrl = $('#chatwootWebhookUrl').val();
    if (webhookUrl && webhookUrl !== 'Will be generated after saving...') {
      navigator.clipboard.writeText(webhookUrl).then(() => {
        showSuccess('Webhook URL copied to clipboard!');
      });
    }
  });

  // Save button handler
  $('#saveChatwootConfig').on('click', function() {
    saveChatwootConfig();
  });

  // Delete button handler
  $('#deleteChatwootConfig').on('click', function() {
    deleteChatwootConfig();
  });
}

/**
 * Open Chatwoot configuration modal
 */
function openChatwootConfig() {
  $('#modalChatwootConfig').modal({
    onShow: function() {
      loadChatwootConfig();
    }
  }).modal('show');
}

/**
 * Load Chatwoot configuration from server
 */
async function loadChatwootConfig() {
  const token = getLocalStorageItem('token');
  if (!token) {
    showError('No authentication token found');
    return;
  }

  try {
    const response = await fetch(baseUrl + '/chatwoot/config', {
      method: 'GET',
      headers: {
        'token': token,
        'Content-Type': 'application/json'
      }
    });

    if (response.status === 404) {
      // No configuration yet - show empty form with defaults
      $('#chatwootEnabled').prop('checked', false);
      $('#chatwootAutoCreate').prop('checked', true);
      $('#chatwootUrl').val('');
      $('#chatwootAccountId').val('');
      $('#chatwootToken').val('');
      $('#chatwootInboxName').val('Wuzapi');
      $('#chatwootSignMsg').prop('checked', false);
      $('#chatwootSignDelimiter').val('\\n');
      $('#chatwootReopenConversation').prop('checked', false);
      $('#chatwootConversationPending').prop('checked', false);
      $('#chatwootMergeBrazilContacts').prop('checked', false);
      $('#chatwootOrganization').val('');
      $('#chatwootLogo').val('');
      
      // Hide delete button and status
      $('#deleteChatwootConfig').hide();
      $('#chatwootCurrentStatus').hide();
      
      // Generate webhook URL with current instance
      const urrent = getLocalStorageItem('currentInstance');
      const webhookUrl = `${window.location.origin}/chatwoot/webhook/${token}`;
      $('#chatwootWebhookUrl').val(webhookUrl);
      
      return;
    }

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}: ${await response.text()}`);
    }

    const data = await response.json();
    
    // Populate form fields
    $('#chatwootEnabled').prop('checked', data.enabled);
    $('#chatwootUrl').val(data.url);
    $('#chatwootAccountId').val(data.account_id);
    
    // Show masked token (don't allow editing if already set)
    if (data.token) {
      $('#chatwootToken').val(data.token);
      $('#chatwootToken').attr('placeholder', 'Keep empty to not change');
    }
    
    $('#chatwootInboxName').val(data.name_inbox || 'Wuzapi');
    $('#chatwootAutoCreate').prop('checked', data.auto_create);
    $('#chatwootSignMsg').prop('checked', data.sign_msg);
    $('#chatwootSignDelimiter').val(data.sign_delimiter || '\\n');
    $('#chatwootReopenConversation').prop('checked', data.reopen_conversation);
    $('#chatwootConversationPending').prop('checked', data.conversation_pending);
    $('#chatwootMergeBrazilContacts').prop('checked', data.merge_brazil_contacts);
    $('#chatwootOrganization').val(data.organization || '');
    $('#chatwootLogo').val(data.logo || '');
    
    // Set webhook URL (read-only)
    $('#chatwootWebhookUrl').val(data.webhook_url);
    
    // Show current status if inbox exists
    if (data.inbox_id) {
      $('#chatwootCurrentInboxId').text(data.inbox_id);
      $('#chatwootCurrentEnabled').html(
        data.enabled 
          ? '<span style="color: green;">Enabled</span>' 
          : '<span style="color: red;">Disabled</span>'
      );
      $('#chatwootCurrentUpdated').text(new Date(data.updated_at).toLocaleString());
      $('#chatwootCurrentStatus').show();
      $('#deleteChatwootConfig').show();
    } else {
      $('#chatwootCurrentStatus').hide();
      $('#deleteChatwootConfig').hide();
    }

  } catch (error) {
    console.error('Error loading Chatwoot config:', error);
    // Don't show error for 404 - it's expected on first setup
    if (error.message && !error.message.includes('404')) {
      showError('Failed to load Chatwoot configuration: ' + error.message);
    }
  }
}

/**
 * Save Chatwoot configuration to server
 */
async function saveChatwootConfig() {
  const token = getLocalStorageItem('token');
  if (!token) {
    showError('No authentication token found');
    return;
  }

  // Validate required fields
  const url = $('#chatwootUrl').val().trim();
  const accountId = $('#chatwootAccountId').val().trim();
  const accessToken = $('#chatwootToken').val().trim();

  if (!url || !accountId) {
    showError('Please fill in all required fields (URL, Account ID)');
    return;
  }

  // Only require token if it's not masked (new setup or changing token)
  if (!accessToken || accessToken.startsWith('****')) {
    if (!accessToken || accessToken === '') {
      showError('Please provide an access token');
      return;
    }
  }

  // Build configuration object
  const config = {
    url: url,
    account_id: accountId,
    token: accessToken,
    name_inbox: $('#chatwootInboxName').val().trim() || 'Wuzapi',
    enabled: $('#chatwootEnabled').is(':checked'),
    auto_create: $('#chatwootAutoCreate').is(':checked'),
    sign_msg: $('#chatwootSignMsg').is(':checked'),
    sign_delimiter: $('#chatwootSignDelimiter').val() || '\\n',
    reopen_conversation: $('#chatwootReopenConversation').is(':checked'),
    conversation_pending: $('#chatwootConversationPending').is(':checked'),
    merge_brazil_contacts: $('#chatwootMergeBrazilContacts').is(':checked'),
    organization: $('#chatwootOrganization').val().trim() || '',
    logo: $('#chatwootLogo').val().trim() || ''
  };

  // Show loading state
  const $saveBtn = $('#saveChatwootConfig');
  $saveBtn.addClass('loading disabled');

  try {
    const response = await fetch(baseUrl + '/chatwoot/config', {
      method: 'POST',
      headers: {
        'token': token,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(config)
    });

    const data = await response.json();

    if (!response.ok) {
      throw new Error(data.error || data.message || `HTTP ${response.status}`);
    }

    // Success!
    showSuccess('Chatwoot configured successfully!' + (data.inbox_id ? ` Inbox ID: ${data.inbox_id}` : ''));
    
    // Close modal and reload config to show updated status
    setTimeout(() => {
      $('#modalChatwootConfig').modal('hide');
    }, 1000);

  } catch (error) {
    console.error('Error saving Chatwoot config:', error);
    showError('Failed to save configuration: ' + error.message);
  } finally {
    $saveBtn.removeClass('loading disabled');
  }
}

/**
 * Delete Chatwoot configuration
 */
async function deleteChatwootConfig() {
  if (!confirm('Are you sure you want to delete the Chatwoot configuration? This action cannot be undone.')) {
    return;
  }

  const token = getLocalStorageItem('token');
  if (!token) {
    showError('No authentication token found');
    return;
  }

  const $deleteBtn = $('#deleteChatwootConfig');
  $deleteBtn.addClass('loading disabled');

  try {
    const response = await fetch(baseUrl + '/chatwoot/config', {
      method: 'DELETE',
      headers: {
        'token': token,
        'Content-Type': 'application/json'
      }
    });

    const data = await response.json();

    if (!response.ok) {
      throw new Error(data.error || data.message || `HTTP ${response.status}`);
    }

    showSuccess('Chatwoot configuration deleted successfully');
    
    // Close modal
    setTimeout(() => {
      $('#modalChatwootConfig').modal('hide');
    }, 1000);

  } catch (error) {
    console.error('Error deleting Chatwoot config:', error);
    showError('Failed to delete configuration: ' + error.message);
  } finally {
    $deleteBtn.removeClass('loading disabled');
  }
}
