package com.automatedazan.service

import android.util.Log
import java.time.LocalTime
import java.time.format.DateTimeFormatter

/**
 * Service to log prayer time notifications (Twilio functionality removed)
 */
class WhatsAppNotificationService {
    companion object {
        private const val TAG = "WhatsAppNotify"
    }

    private val timeFormatter = DateTimeFormatter.ofPattern("HH:mm")

    /**
     * Logs a prayer time notification (stub implementation)
     * @param prayerName Name of the prayer (e.g., "Fajr", "Dhuhr")
     * @param prayerTime Time of the prayer
     * @return always returns true since no actual network operation is performed
     */
    suspend fun sendNotification(prayerName: String, prayerTime: LocalTime): Boolean {
        val currentTime = LocalTime.now().format(timeFormatter)
        val scheduledTime = prayerTime.format(timeFormatter)
        
        Log.i(TAG, "Prayer notification for $prayerName at $scheduledTime (current time: $currentTime)")
        
        // Always return success since we're just logging
        return true
    }
}