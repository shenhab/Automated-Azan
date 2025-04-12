package com.automatedazan.data.repository

import android.content.Context
import android.util.Log
import com.automatedazan.data.PreferenceManager
import com.automatedazan.data.model.PrayerTime
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.flowOn
import org.json.JSONObject
import java.io.File
import java.io.IOException
import java.net.URL
import java.time.LocalDate
import java.time.format.DateTimeFormatter
import javax.net.ssl.HttpsURLConnection
import okhttp3.OkHttpClient
import okhttp3.Request

/**
 * Implementation of PrayerTimeRepository that fetches prayer times from ICCI and NAAS sources
 */
class PrayerTimeRepositoryImpl(private val context: Context) : PrayerTimeRepository {
    
    companion object {
        private const val TAG = "PrayerTimeRepository"
        private const val ICCI_URL = "https://islamireland.ie/api/timetable/"
        private const val NAAS_URL = "https://mawaqit.net/en/m/-34"
        
        private const val ICCI_TIMETABLE_FILE = "icci_timetable.json"
        private const val NAAS_TIMETABLE_FILE = "naas_prayers_timetable.json"
    }
    
    private val httpClient = OkHttpClient()
    
    override fun getTodayPrayerTimes(): Flow<Result<PrayerTime>> = flow {
        try {
            // Check if we need to download new timetables
            if (isNewMonth()) {
                Log.d(TAG, "New month detected or files missing, downloading timetables")
                downloadTimetables()
            }
            
            val location = PreferenceManager.getPrayerLocation()
            Log.d(TAG, "Fetching prayer times for location: $location")
            
            val prayerMap = when (location) {
                PreferenceManager.LOCATION_ICCI -> extractIcciPrayerTimes()
                PreferenceManager.LOCATION_NAAS -> extractNaasPrayerTimes()
                else -> throw IllegalArgumentException("Invalid location: $location")
            }
            
            val prayerTime = PrayerTime.fromMap(prayerMap)
            emit(Result.success(prayerTime))
        } catch (e: Exception) {
            Log.e(TAG, "Error fetching prayer times", e)
            emit(Result.failure(e))
        }
    }.flowOn(Dispatchers.IO)
    
    override fun refreshPrayerTimes(): Flow<Result<PrayerTime>> = flow {
        try {
            Log.d(TAG, "Forcing refresh of prayer times")
            downloadTimetables()
            
            // After download, get today's times
            val result = getTodayPrayerTimes()
            result.collect { emit(it) }
        } catch (e: Exception) {
            Log.e(TAG, "Error refreshing prayer times", e)
            emit(Result.failure(e))
        }
    }.flowOn(Dispatchers.IO)
    
    /**
     * Determines if we need to download new timetables (new month or missing files)
     */
    private fun isNewMonth(): Boolean {
        val icciFile = File(context.filesDir, ICCI_TIMETABLE_FILE)
        val naasFile = File(context.filesDir, NAAS_TIMETABLE_FILE)
        
        // If files don't exist, we need to download them
        if (!icciFile.exists() || !naasFile.exists()) {
            return true
        }
        
        // Check if it's a new month by comparing file modification date
        val fileMonth = LocalDate.ofEpochDay(icciFile.lastModified() / (24 * 60 * 60 * 1000)).monthValue
        val currentMonth = LocalDate.now().monthValue
        
        return fileMonth != currentMonth
    }
    
    /**
     * Downloads timetables from both sources
     */
    private suspend fun downloadTimetables() {
        try {
            downloadIcciTimetable()
            downloadNaasTimetable()
        } catch (e: Exception) {
            Log.e(TAG, "Error downloading timetables", e)
            throw e
        }
    }
    
    /**
     * Downloads ICCI timetable from API
     */
    private fun downloadIcciTimetable() {
        try {
            val request = Request.Builder()
                .url(ICCI_URL)
                .build()
            
            httpClient.newCall(request).execute().use { response ->
                if (!response.isSuccessful) {
                    throw IOException("Failed to download ICCI timetable: ${response.code}")
                }
                
                val jsonString = response.body?.string()
                if (jsonString != null) {
                    // Save to file
                    File(context.filesDir, ICCI_TIMETABLE_FILE).writeText(jsonString)
                    Log.d(TAG, "ICCI timetable downloaded successfully")
                }
            }
        } catch (e: Exception) {
            Log.e(TAG, "Error downloading ICCI timetable", e)
            throw e
        }
    }
    
    /**
     * Downloads NAAS timetable via web scraping
     * This is a simplified version - in a real app, you'd use a HTML parsing library
     */
    private fun downloadNaasTimetable() {
        try {
            val request = Request.Builder()
                .url(NAAS_URL)
                .build()
            
            httpClient.newCall(request).execute().use { response ->
                if (!response.isSuccessful) {
                    throw IOException("Failed to download NAAS page: ${response.code}")
                }
                
                val html = response.body?.string()
                if (html != null) {
                    // Simple extraction of calendar data from script tag
                    val calendarDataRegex = "\"calendar\"\\s*:\\s*(\\[\\{.*?\\}\\])".toRegex(RegexOption.DOT_MATCHES_ALL)
                    val match = calendarDataRegex.find(html)
                    
                    if (match != null) {
                        val calendarData = match.groupValues[1]
                        File(context.filesDir, NAAS_TIMETABLE_FILE).writeText(calendarData)
                        Log.d(TAG, "NAAS timetable extracted and saved successfully")
                    } else {
                        throw IOException("Failed to extract calendar data from NAAS webpage")
                    }
                }
            }
        } catch (e: Exception) {
            Log.e(TAG, "Error downloading NAAS timetable", e)
            throw e
        }
    }
    
    /**
     * Extracts today's prayer times from ICCI timetable
     */
    private fun extractIcciPrayerTimes(): Map<String, String> {
        val today = LocalDate.now()
        val todayMonth = today.monthValue.toString()
        val todayDay = today.dayOfMonth.toString()
        
        try {
            val jsonString = File(context.filesDir, ICCI_TIMETABLE_FILE).readText()
            val jsonObject = JSONObject(jsonString)
            
            val timetable = jsonObject.getJSONObject("timetable")
            val monthData = timetable.getJSONObject(todayMonth)
            val dayData = monthData.getJSONArray(todayDay)
            
            // Format: [hour, minute] arrays for each prayer
            return mapOf(
                "Fajr" to String.format("%02d:%02d", dayData.getJSONArray(0).getInt(0), dayData.getJSONArray(0).getInt(1)),
                "Dhuhr" to String.format("%02d:%02d", dayData.getJSONArray(2).getInt(0), dayData.getJSONArray(2).getInt(1)),
                "Asr" to String.format("%02d:%02d", dayData.getJSONArray(3).getInt(0), dayData.getJSONArray(3).getInt(1)),
                "Maghrib" to String.format("%02d:%02d", dayData.getJSONArray(4).getInt(0), dayData.getJSONArray(4).getInt(1)),
                "Isha" to String.format("%02d:%02d", dayData.getJSONArray(5).getInt(0), dayData.getJSONArray(5).getInt(1))
            )
        } catch (e: Exception) {
            Log.e(TAG, "Error extracting ICCI prayer times", e)
            throw e
        }
    }
    
    /**
     * Extracts today's prayer times from NAAS timetable
     */
    private fun extractNaasPrayerTimes(): Map<String, String> {
        val today = LocalDate.now()
        val todayDay = today.dayOfMonth.toString()
        val month = today.monthValue - 1 // 0-based month index
        
        try {
            val jsonString = File(context.filesDir, NAAS_TIMETABLE_FILE).readText()
            val jsonArray = org.json.JSONArray(jsonString)
            
            if (month < 0 || month >= jsonArray.length()) {
                throw IOException("Month index out of bounds")
            }
            
            val monthData = jsonArray.getJSONObject(month)
            if (!monthData.has(todayDay)) {
                throw IOException("Day not found in month data")
            }
            
            val dayData = monthData.getJSONArray(todayDay)
            
            return mapOf(
                "Fajr" to dayData.getString(0),
                "Dhuhr" to dayData.getString(2),
                "Asr" to dayData.getString(3),
                "Maghrib" to dayData.getString(4),
                "Isha" to dayData.getString(5)
            )
        } catch (e: Exception) {
            Log.e(TAG, "Error extracting NAAS prayer times", e)
            throw e
        }
    }
}