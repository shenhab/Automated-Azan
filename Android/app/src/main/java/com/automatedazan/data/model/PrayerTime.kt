package com.automatedazan.data.model

import java.time.LocalDate
import java.time.LocalTime
import java.time.format.DateTimeFormatter

/**
 * Data class representing prayer times for a specific day
 */
data class PrayerTime(
    val date: LocalDate = LocalDate.now(),
    val fajr: LocalTime,
    val dhuhr: LocalTime,
    val asr: LocalTime,
    val maghrib: LocalTime,
    val isha: LocalTime
) {
    companion object {
        private val TIME_FORMATTER = DateTimeFormatter.ofPattern("HH:mm")
        
        /**
         * Creates a PrayerTime object from a map of prayer names to time strings (HH:MM format)
         */
        fun fromMap(prayerMap: Map<String, String>): PrayerTime {
            return PrayerTime(
                fajr = LocalTime.parse(prayerMap["Fajr"] ?: "00:00", TIME_FORMATTER),
                dhuhr = LocalTime.parse(prayerMap["Dhuhr"] ?: "00:00", TIME_FORMATTER),
                asr = LocalTime.parse(prayerMap["Asr"] ?: "00:00", TIME_FORMATTER),
                maghrib = LocalTime.parse(prayerMap["Maghrib"] ?: "00:00", TIME_FORMATTER),
                isha = LocalTime.parse(prayerMap["Isha"] ?: "00:00", TIME_FORMATTER)
            )
        }
    }
    
    /**
     * Converts the prayer time to a map of prayer names to formatted time strings
     */
    fun toMap(): Map<String, String> {
        return mapOf(
            "Fajr" to fajr.format(TIME_FORMATTER),
            "Dhuhr" to dhuhr.format(TIME_FORMATTER),
            "Asr" to asr.format(TIME_FORMATTER),
            "Maghrib" to maghrib.format(TIME_FORMATTER),
            "Isha" to isha.format(TIME_FORMATTER)
        )
    }
    
    /**
     * Returns a list of prayer names and times for iterating through all prayers
     */
    fun getPrayers(): List<Pair<String, LocalTime>> {
        return listOf(
            "Fajr" to fajr,
            "Dhuhr" to dhuhr,
            "Asr" to asr,
            "Maghrib" to maghrib,
            "Isha" to isha
        )
    }
    
    /**
     * Gets the next prayer time after the provided time
     * @return Pair of prayer name and time, or null if no prayers remain today
     */
    fun getNextPrayerAfter(currentTime: LocalTime): Pair<String, LocalTime>? {
        return getPrayers().find { (_, time) -> time.isAfter(currentTime) }
    }
}