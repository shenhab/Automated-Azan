package com.automatedazan

import android.app.Application
import android.content.Context
import androidx.core.app.NotificationChannelCompat
import androidx.core.app.NotificationManagerCompat
import androidx.work.Configuration
import androidx.work.WorkManager
import com.automatedazan.data.PreferenceManager
import com.automatedazan.worker.PrayerTimesWorkerFactory

class AutomatedAzanApp : Application(), Configuration.Provider {

    companion object {
        const val PRAYER_NOTIFICATION_CHANNEL_ID = "prayer_notifications"
        const val AZAN_PLAYBACK_CHANNEL_ID = "azan_playback"
    }
    
    override fun onCreate() {
        super.onCreate()
        
        // Initialize preferences
        PreferenceManager.init(this)
        
        // Create notification channels
        createNotificationChannels()
        
        // Initialize WorkManager with our custom factory
        WorkManager.initialize(
            this,
            workManagerConfiguration
        )
    }
    
    private fun createNotificationChannels() {
        val notificationManager = NotificationManagerCompat.from(this)
        
        // Prayer notifications channel
        val prayerChannel = NotificationChannelCompat.Builder(
            PRAYER_NOTIFICATION_CHANNEL_ID,
            NotificationManagerCompat.IMPORTANCE_HIGH
        )
            .setName("Prayer Notifications")
            .setDescription("Notifications for upcoming prayers")
            .build()
            
        // Azan playback channel
        val azanChannel = NotificationChannelCompat.Builder(
            AZAN_PLAYBACK_CHANNEL_ID,
            NotificationManagerCompat.IMPORTANCE_LOW
        )
            .setName("Azan Playback")
            .setDescription("Controls for Azan playback on Google Home")
            .build()
            
        notificationManager.createNotificationChannelsCompat(listOf(prayerChannel, azanChannel))
    }
    
    override val workManagerConfiguration: Configuration
        get() = Configuration.Builder()
            .setWorkerFactory(PrayerTimesWorkerFactory())
            .build()
}