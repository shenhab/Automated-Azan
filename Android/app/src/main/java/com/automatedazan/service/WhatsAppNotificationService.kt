package com.automatedazan.service

import android.util.Log
import com.automatedazan.data.PreferenceManager
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.FormBody
import okhttp3.OkHttpClient
import okhttp3.Request
import java.time.LocalTime
import java.time.format.DateTimeFormatter

/**
 * Service to send WhatsApp notifications via Twilio API
 */
class WhatsAppNotificationService {
    companion object {
        private const val TAG = "WhatsAppNotify"
        private const val TWILIO_API_URL = "https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json"
    }

    private val httpClient = OkHttpClient()
    private val timeFormatter = DateTimeFormatter.ofPattern("HH:mm")

    /**
     * Sends a WhatsApp notification for a prayer time
     * @param prayerName Name of the prayer (e.g., "Fajr", "Dhuhr")
     * @param prayerTime Time of the prayer
     * @return true if notification was sent successfully
     */
    suspend fun sendNotification(prayerName: String, prayerTime: LocalTime): Boolean = withContext(Dispatchers.IO) {
        // Check if WhatsApp notifications are enabled
        if (!PreferenceManager.isWhatsAppEnabled() || !PreferenceManager.isTwilioConfigured()) {
            Log.d(TAG, "WhatsApp notifications disabled or not configured")
            return@withContext false
        }

        try {
            // Get Twilio credentials
            val accountSid = PreferenceManager.getTwilioAccountSid() ?: return@withContext false
            val authToken = PreferenceManager.getTwilioAuthToken() ?: return@withContext false
            val contentSid = PreferenceManager.getTwilioContentSid() ?: return@withContext false
            val twilioWhatsAppNumber = PreferenceManager.getTwilioWhatsAppNumber() ?: return@withContext false
            val recipientNumber = PreferenceManager.getRecipientNumber() ?: return@withContext false

            // Current time for the notification
            val currentTime = LocalTime.now().format(timeFormatter)
            val scheduledTime = prayerTime.format(timeFormatter)

            // Create request body
            val requestBody = FormBody.Builder()
                .add("From", twilioWhatsAppNumber)
                .add("To", recipientNumber)
                .add("ContentSid", contentSid)
                .add("ContentVariables", """{"1":"$prayerName","2":"$scheduledTime","3":"$currentTime"}""")
                .build()

            // Build the request with Basic Auth
            val request = Request.Builder()
                .url(String.format(TWILIO_API_URL, accountSid))
                .addHeader("Authorization", okhttp3.Credentials.basic(accountSid, authToken))
                .post(requestBody)
                .build()

            // Execute request
            httpClient.newCall(request).execute().use { response ->
                val isSuccessful = response.isSuccessful
                if (isSuccessful) {
                    Log.i(TAG, "WhatsApp notification sent successfully for $prayerName")
                } else {
                    Log.e(TAG, "Failed to send WhatsApp notification: ${response.code} - ${response.message}")
                }
                return@withContext isSuccessful
            }
        } catch (e: Exception) {
            Log.e(TAG, "Error sending WhatsApp notification", e)
            return@withContext false
        }
    }
}