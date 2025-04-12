package com.automatedazan.worker

import android.content.Context
import androidx.work.ListenableWorker
import androidx.work.WorkerFactory
import androidx.work.WorkerParameters

/**
 * Factory to create prayer times workers with the required dependencies
 */
class PrayerTimesWorkerFactory : WorkerFactory() {
    override fun createWorker(
        appContext: Context,
        workerClassName: String,
        workerParameters: WorkerParameters
    ): ListenableWorker? {
        return when (workerClassName) {
            PrayerTimesSchedulerWorker::class.java.name -> {
                PrayerTimesSchedulerWorker(appContext, workerParameters)
            }
            PrayerNotificationWorker::class.java.name -> {
                PrayerNotificationWorker(appContext, workerParameters)
            }
            else -> null
        }
    }
}