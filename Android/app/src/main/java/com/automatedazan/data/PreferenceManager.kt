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
     * Check if all required settings are configured
     */
    fun isSetupComplete(): Boolean {
        // Only Cast device name is required now
        return !getCastDeviceName().isNullOrEmpty()
    }
}