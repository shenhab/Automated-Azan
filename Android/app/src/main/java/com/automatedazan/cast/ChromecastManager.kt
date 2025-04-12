package com.automatedazan.cast

import android.content.Context
import android.net.Uri
import android.util.Log
import com.automatedazan.data.PreferenceManager
import com.google.android.gms.cast.MediaInfo
import com.google.android.gms.cast.MediaLoadOptions
import com.google.android.gms.cast.MediaMetadata
import com.google.android.gms.cast.framework.CastContext
import com.google.android.gms.cast.framework.CastSession
import com.google.android.gms.cast.framework.SessionManager
import com.google.android.gms.common.images.WebImage
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/**
 * Manages Chromecast connection and media playback for Azan
 */
class ChromecastManager(private val context: Context) {

    companion object {
        private const val TAG = "ChromecastManager"
        
        // Audio URLs
        private const val REGULAR_AZAN_URL = "https://server8.mp3quran.net/azan/Alafasy_Azan.mp3"
        private const val FAJR_AZAN_URL = "https://server8.mp3quran.net/azan/Alafasy_Fajr.mp3"
        private const val QURAN_RADIO_URL = "https://server14.mp3quran.net/islam/Rewayat-Hafs-A-n-Assem/001.mp3"
    }
    
    private var sessionManager: SessionManager? = null
    private var castSession: CastSession? = null
    
    init {
        try {
            val castContext = CastContext.getSharedInstance(context)
            sessionManager = castContext.sessionManager
        } catch (e: Exception) {
            Log.e(TAG, "Error initializing Cast context", e)
        }
    }
    
    /**
     * Play regular Azan audio on the selected Google Home device
     */
    suspend fun playAzan(): Boolean = withContext(Dispatchers.IO) {
        try {
            if (!connectToCastDevice()) {
                Log.e(TAG, "Failed to connect to Cast device")
                return@withContext false
            }
            
            loadAndPlayMedia(REGULAR_AZAN_URL, "Azan", "Regular Azan")
            return@withContext true
        } catch (e: Exception) {
            Log.e(TAG, "Error playing Azan", e)
            return@withContext false
        }
    }
    
    /**
     * Play Fajr-specific Azan audio on the selected Google Home device
     */
    suspend fun playFajrAzan(): Boolean = withContext(Dispatchers.IO) {
        try {
            if (!connectToCastDevice()) {
                Log.e(TAG, "Failed to connect to Cast device")
                return@withContext false
            }
            
            loadAndPlayMedia(FAJR_AZAN_URL, "Fajr Azan", "Fajr Prayer Call")
            return@withContext true
        } catch (e: Exception) {
            Log.e(TAG, "Error playing Fajr Azan", e)
            return@withContext false
        }
    }
    
    /**
     * Play Quran radio as a wake-up call before Fajr
     */
    suspend fun playQuranRadio(): Boolean = withContext(Dispatchers.IO) {
        try {
            if (!connectToCastDevice()) {
                Log.e(TAG, "Failed to connect to Cast device")
                return@withContext false
            }
            
            loadAndPlayMedia(QURAN_RADIO_URL, "Quran Radio", "Wake-up Quran Recitation")
            return@withContext true
        } catch (e: Exception) {
            Log.e(TAG, "Error playing Quran Radio", e)
            return@withContext false
        }
    }
    
    /**
     * Connect to the configured Google Home device
     * @return true if connected successfully
     */
    private fun connectToCastDevice(): Boolean {
        try {
            val deviceName = PreferenceManager.getCastDeviceName()
            if (deviceName.isNullOrEmpty()) {
                Log.e(TAG, "No Cast device configured")
                return false
            }
            
            // Get current cast session
            castSession = sessionManager?.currentCastSession
            
            if (castSession == null || !castSession!!.isConnected) {
                Log.d(TAG, "No active Cast session - user needs to connect manually first")
                return false
            }
            
            return true
        } catch (e: Exception) {
            Log.e(TAG, "Error connecting to Cast device", e)
            return false
        }
    }
    
    /**
     * Load and play media on the connected Cast device
     */
    private fun loadAndPlayMedia(url: String, title: String, subtitle: String): Boolean {
        try {
            castSession?.let { session ->
                val remoteMediaClient = session.remoteMediaClient ?: return false
                
                val mediaMetadata = MediaMetadata(MediaMetadata.MEDIA_TYPE_MUSIC_TRACK).apply {
                    putString(MediaMetadata.KEY_TITLE, title)
                    putString(MediaMetadata.KEY_SUBTITLE, subtitle)
                    addImage(WebImage(Uri.parse("https://upload.wikimedia.org/wikipedia/commons/thumb/e/e2/Mosque_icon.svg/240px-Mosque_icon.svg.png")))
                }
                
                val mediaInfo = MediaInfo.Builder(url)
                    .setStreamType(MediaInfo.STREAM_TYPE_BUFFERED)
                    .setContentType("audio/mp3")
                    .setMetadata(mediaMetadata)
                    .build()
                
                val mediaLoadOptions = MediaLoadOptions.Builder().build()
                
                remoteMediaClient.load(mediaInfo, mediaLoadOptions)
                return true
            }
            
            return false
        } catch (e: Exception) {
            Log.e(TAG, "Error loading media", e)
            return false
        }
    }
}