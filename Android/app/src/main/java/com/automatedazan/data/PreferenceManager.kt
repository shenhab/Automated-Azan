package com.automatedazan.data

import android.content.Context
import android.content.SharedPreferences
import androidx.core.content.edit

/**
 * Manages application preferences and configuration settings
 */
object PreferenceManager {
    private const val PREFS_NAME = "automated_azan_preferences"
    
    // Preference keys
    private const val KEY_PRAYER_LOCATION = "prayer_location"
    private const val KEY_CAST_DEVICE_NAME = "cast_device_name"
    private const val KEY_TWILIO_ACCOUNT_SID = "twilio_account_sid"
    private const val KEY_TWILIO_AUTH_TOKEN = "twilio_auth_token" 
    private const val KEY_TWILIO_CONTENT_SID = "twilio_content_sid"
    private const val KEY_TWILIO_WHATSAPP_NUMBER = "twilio_whatsapp_number"
    private const val KEY_RECIPIENT_NUMBER = "recipient_number"
    private const val KEY_WHATSAPP_ENABLED = "whatsapp_enabled"
    
    // Location options
    const val LOCATION_ICCI = "icci"
    const val LOCATION_NAAS = "naas"
    
    private lateinit var prefs: SharedPreferences
    
    /**
     * Initialize the preference manager
     */
    fun init(context: Context) {
        prefs = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
    }
    
    /**
     * Get the selected prayer time location (ICCI or NAAS)
     */
    fun getPrayerLocation(): String {
        return prefs.getString(KEY_PRAYER_LOCATION, LOCATION_ICCI) ?: LOCATION_ICCI
    }
    
    /**
     * Set the prayer time location
     */
    fun setPrayerLocation(location: String) {
        prefs.edit {
            putString(KEY_PRAYER_LOCATION, location)
        }
    }
    
    /**
     * Get the name of the configured Cast device
     */
    fun getCastDeviceName(): String? {
        return prefs.getString(KEY_CAST_DEVICE_NAME, null)
    }
    
    /**
     * Set the name of the Cast device to use
     */
    fun setCastDeviceName(deviceName: String) {
        prefs.edit {
            putString(KEY_CAST_DEVICE_NAME, deviceName)
        }
    }
    
    /**
     * Check if WhatsApp notifications are enabled
     */
    fun isWhatsAppEnabled(): Boolean {
        return prefs.getBoolean(KEY_WHATSAPP_ENABLED, false)
    }
    
    /**
     * Enable or disable WhatsApp notifications
     */
    fun setWhatsAppEnabled(enabled: Boolean) {
        prefs.edit {
            putBoolean(KEY_WHATSAPP_ENABLED, enabled)
        }
    }
    
    /**
     * Get the Twilio account SID
     */
    fun getTwilioAccountSid(): String? {
        return prefs.getString(KEY_TWILIO_ACCOUNT_SID, null)
    }
    
    /**
     * Set the Twilio account SID
     */
    fun setTwilioAccountSid(sid: String) {
        prefs.edit {
            putString(KEY_TWILIO_ACCOUNT_SID, sid)
        }
    }
    
    /**
     * Get the Twilio auth token
     */
    fun getTwilioAuthToken(): String? {
        return prefs.getString(KEY_TWILIO_AUTH_TOKEN, null)
    }
    
    /**
     * Set the Twilio auth token
     */
    fun setTwilioAuthToken(token: String) {
        prefs.edit {
            putString(KEY_TWILIO_AUTH_TOKEN, token)
        }
    }
    
    /**
     * Get the Twilio content SID
     */
    fun getTwilioContentSid(): String? {
        return prefs.getString(KEY_TWILIO_CONTENT_SID, null)
    }
    
    /**
     * Set the Twilio content SID
     */
    fun setTwilioContentSid(sid: String) {
        prefs.edit {
            putString(KEY_TWILIO_CONTENT_SID, sid)
        }
    }
    
    /**
     * Get the Twilio WhatsApp number
     */
    fun getTwilioWhatsAppNumber(): String? {
        return prefs.getString(KEY_TWILIO_WHATSAPP_NUMBER, null)
    }
    
    /**
     * Set the Twilio WhatsApp number
     */
    fun setTwilioWhatsAppNumber(number: String) {
        prefs.edit {
            putString(KEY_TWILIO_WHATSAPP_NUMBER, number)
        }
    }
    
    /**
     * Get the recipient's WhatsApp number
     */
    fun getRecipientNumber(): String? {
        return prefs.getString(KEY_RECIPIENT_NUMBER, null)
    }
    
    /**
     * Set the recipient's WhatsApp number
     */
    fun setRecipientNumber(number: String) {
        prefs.edit {
            putString(KEY_RECIPIENT_NUMBER, number)
        }
    }
    
    /**
     * Check if Twilio WhatsApp notification settings are complete
     */
    fun isTwilioConfigured(): Boolean {
        return !getTwilioAccountSid().isNullOrEmpty() &&
               !getTwilioAuthToken().isNullOrEmpty() &&
               !getTwilioContentSid().isNullOrEmpty() &&
               !getTwilioWhatsAppNumber().isNullOrEmpty() &&
               !getRecipientNumber().isNullOrEmpty()
    }
    
    /**
     * Check if all required settings are configured
     */
    fun isSetupComplete(): Boolean {
        // Cast device name is required
        if (getCastDeviceName().isNullOrEmpty()) {
            return false
        }
        
        // If WhatsApp is enabled, check Twilio config
        if (isWhatsAppEnabled() && !isTwilioConfigured()) {
            return false
        }
        
        return true
    }
}