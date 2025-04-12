package com.automatedazan.data.repository

import com.automatedazan.data.model.PrayerTime
import kotlinx.coroutines.flow.Flow

/**
 * Repository interface for fetching prayer times
 */
interface PrayerTimeRepository {
    /**
     * Fetch prayer times for today
     * @return Flow emitting today's prayer times or error
     */
    fun getTodayPrayerTimes(): Flow<Result<PrayerTime>>
    
    /**
     * Force refresh prayer times data from the source
     * @return Flow emitting success or error
     */
    fun refreshPrayerTimes(): Flow<Result<PrayerTime>>
}