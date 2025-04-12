package com.automatedazan.worker

import android.app.Notification
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.util.Log
import androidx.core.app.NotificationCompat
import androidx.work.CoroutineWorker
import androidx.work.ForegroundInfo
import androidx.work.WorkerParameters
import com.automatedazan.AutomatedAzanApp
import com.automatedazan.R
import com.automatedazan.cast.ChromecastManager
import com.automatedazan.ui.MainActivity
import java.time.LocalTime
import java.time.format.DateTimeFormatter

/**
 * Worker that executes at prayer time to play Azan and send notifications
 */
class PrayerNotificationWorker(
    private val appContext: Context,
    params: WorkerParameters
) : CoroutineWorker(appContext, params) {

    companion object {
        private const val TAG = "PrayerNotifyWorker"
        private const val NOTIFICATION_ID = 1001
    }
    
    override suspend fun doWork(): Result {
        val prayerName = inputData.getString(PrayerTimesSchedulerWorker.KEY_PRAYER_NAME) ?: return Result.failure()
        val isFajr = inputData.getBoolean(PrayerTimesSchedulerWorker.KEY_IS_FAJR, false)
        val isWakeupCall = inputData.getBoolean(PrayerTimesSchedulerWorker.KEY_WAKEUPCALL_TIME, false)
        
        Log.i(TAG, "PrayerNotificationWorker executing for $prayerName, isFajr=$isFajr, isWakeupCall=$isWakeupCall")
        
        try {
            // Create and show notification
            setForeground(createForegroundInfo(prayerName))
            
            // Play appropriate audio based on prayer type
            val chromecastManager = ChromecastManager(appContext)
            
            when {
                isWakeupCall -> {
                    // Play Quran radio for wake-up call before Fajr
                    chromecastManager.playQuranRadio()
                }
                isFajr -> {
                    // Play Fajr Azan
                    chromecastManager.playFajrAzan()
                }
                else -> {
                    // Play regular Azan
                    chromecastManager.playAzan()
                }
            }
            
            return Result.success()
        } catch (e: Exception) {
            Log.e(TAG, "Error in PrayerNotificationWorker", e)
            return Result.failure()
        }
    }
    
    /**
     * Create notification for foreground service
     */
    private fun createForegroundInfo(prayerName: String): ForegroundInfo {
        val title = if (prayerName == "Fajr") "Fajr Prayer Time" else "$prayerName Prayer Time"
        val currentTime = LocalTime.now().format(DateTimeFormatter.ofPattern("HH:mm"))
        
        // Create a notification channel
        val intent = Intent(appContext, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK
        }
        
        val pendingIntent = PendingIntent.getActivity(
            appContext, 0, intent, PendingIntent.FLAG_IMMUTABLE
        )
        
        val notification = NotificationCompat.Builder(
            appContext, AutomatedAzanApp.PRAYER_NOTIFICATION_CHANNEL_ID
        )
            .setContentTitle(title)
            .setContentText("It's time for $prayerName prayer at $currentTime")
            .setSmallIcon(R.drawable.ic_notification)
            .setContentIntent(pendingIntent)
            .setOngoing(true)
            .setCategory(Notification.CATEGORY_ALARM)
            .setPriority(NotificationCompat.PRIORITY_HIGH)
            .build()
            
        return ForegroundInfo(NOTIFICATION_ID, notification)
    }
}