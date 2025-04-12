package com.automatedazan.worker

import android.content.Context
import android.util.Log
import androidx.work.*
import com.automatedazan.data.model.PrayerTime
import com.automatedazan.data.repository.PrayerTimeRepositoryImpl
import java.time.Duration
import java.time.LocalDate
import java.time.LocalDateTime
import java.time.LocalTime
import java.time.ZoneId
import java.util.concurrent.TimeUnit

/**
 * Worker that runs daily to schedule all prayer notifications for the day
 */
class PrayerTimesSchedulerWorker(
    appContext: Context,
    params: WorkerParameters
) : CoroutineWorker(appContext, params) {

    companion object {
        private const val TAG = "PrayerScheduler"
        private const val UNIQUE_WORK_NAME = "prayer_scheduler_daily"
        
        // Keys for worker data
        const val KEY_PRAYER_NAME = "prayer_name"
        const val KEY_IS_FAJR = "is_fajr"
        const val KEY_WAKEUPCALL_TIME = "wakeup_call_time"
        
        /**
         * Schedule this worker to run every day at 1:00 AM
         */
        fun schedule(context: Context) {
            val currentDate = LocalDate.now()
            val targetTime = LocalTime.of(1, 0) // 1:00 AM
            
            // Calculate the initial delay until 1:00 AM tomorrow
            var targetDateTime = LocalDateTime.of(currentDate, targetTime)
            if (LocalDateTime.now().isAfter(targetDateTime)) {
                targetDateTime = targetDateTime.plusDays(1)
            }
            
            val initialDelay = Duration.between(
                LocalDateTime.now(),
                targetDateTime
            ).toMillis()
            
            val schedulerRequest = PeriodicWorkRequestBuilder<PrayerTimesSchedulerWorker>(
                24, TimeUnit.HOURS, // Run every 24 hours
                23, TimeUnit.HOURS // With a 1-hour flex period
            )
                .setInitialDelay(initialDelay, TimeUnit.MILLISECONDS)
                .setBackoffCriteria(
                    BackoffPolicy.LINEAR,
                    10, TimeUnit.MINUTES
                )
                .build()
                
            WorkManager.getInstance(context).enqueueUniquePeriodicWork(
                UNIQUE_WORK_NAME,
                ExistingPeriodicWorkPolicy.REPLACE,
                schedulerRequest
            )
            
            Log.i(TAG, "Scheduled daily prayer time sync at 1:00 AM with initial delay: ${initialDelay/1000/60} minutes")
        }
    }
    
    override suspend fun doWork(): Result {
        Log.i(TAG, "Starting daily prayer times scheduler")
        
        try {
            // Get today's prayer times
            val repository = PrayerTimeRepositoryImpl(applicationContext)
            val prayerTimesResult = repository.refreshPrayerTimes().collect { result ->
                if (result.isSuccess) {
                    val prayerTimes = result.getOrNull()
                    if (prayerTimes != null) {
                        schedulePrayersForToday(prayerTimes)
                    } else {
                        Log.e(TAG, "Prayer times are null after refresh")
                        return@collect
                    }
                } else {
                    Log.e(TAG, "Failed to refresh prayer times", result.exceptionOrNull())
                    return@collect
                }
            }
            
            return Result.success()
        } catch (e: Exception) {
            Log.e(TAG, "Error in prayer scheduler worker", e)
            return Result.retry()
        }
    }
    
    /**
     * Schedule notification workers for each prayer time
     */
    private fun schedulePrayersForToday(prayerTime: PrayerTime) {
        Log.d(TAG, "Scheduling prayers for today: ${prayerTime.toMap()}")
        
        val now = LocalTime.now()
        var scheduledCount = 0
        
        // Schedule each prayer that's in the future
        prayerTime.getPrayers().forEach { (prayerName, time) ->
            if (time.isAfter(now)) {
                // Calculate delay until prayer time
                val delayMillis = Duration.between(
                    LocalDateTime.now(),
                    LocalDateTime.of(LocalDate.now(), time)
                ).toMillis()
                
                // Schedule the prayer notification
                val isFajr = prayerName == "Fajr"
                schedulePrayerNotification(prayerName, time, isFajr, delayMillis)
                
                // For Fajr, also schedule a wake-up call 45 minutes before
                if (isFajr) {
                    val wakeupTime = time.minusMinutes(45)
                    if (wakeupTime.isAfter(now)) {
                        val wakeupDelayMillis = Duration.between(
                            LocalDateTime.now(),
                            LocalDateTime.of(LocalDate.now(), wakeupTime)
                        ).toMillis()
                        
                        scheduleWakeUpCall(prayerName, wakeupTime, wakeupDelayMillis)
                    }
                }
                
                scheduledCount++
            }
        }
        
        Log.i(TAG, "Scheduled $scheduledCount prayer notifications for today")
    }
    
    /**
     * Schedule a prayer notification worker
     */
    private fun schedulePrayerNotification(
        prayerName: String,
        prayerTime: LocalTime,
        isFajr: Boolean,
        delayMillis: Long
    ) {
        val data = workDataOf(
            KEY_PRAYER_NAME to prayerName,
            KEY_IS_FAJR to isFajr
        )
        
        val notificationWork = OneTimeWorkRequestBuilder<PrayerNotificationWorker>()
            .setInputData(data)
            .setInitialDelay(delayMillis, TimeUnit.MILLISECONDS)
            .addTag("prayer_notification_$prayerName")
            .build()
            
        WorkManager.getInstance(applicationContext)
            .enqueue(notificationWork)
            
        Log.d(TAG, "Scheduled $prayerName notification at ${prayerTime.toString()} (delay: ${delayMillis/1000/60} minutes)")
    }
    
    /**
     * Schedule a wake-up call 45 minutes before Fajr
     */
    private fun scheduleWakeUpCall(
        prayerName: String,
        wakeupTime: LocalTime,
        delayMillis: Long
    ) {
        val data = workDataOf(
            KEY_PRAYER_NAME to prayerName,
            KEY_IS_FAJR to true,
            KEY_WAKEUPCALL_TIME to true
        )
        
        val wakeupWork = OneTimeWorkRequestBuilder<PrayerNotificationWorker>()
            .setInputData(data)
            .setInitialDelay(delayMillis, TimeUnit.MILLISECONDS)
            .addTag("wakeup_call_$prayerName")
            .build()
            
        WorkManager.getInstance(applicationContext)
            .enqueue(wakeupWork)
            
        Log.d(TAG, "Scheduled wake-up call before $prayerName at ${wakeupTime.toString()} (delay: ${delayMillis/1000/60} minutes)")
    }
}