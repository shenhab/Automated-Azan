package com.automatedazan.cast

import android.content.Context
import com.google.android.gms.cast.framework.CastOptions
import com.google.android.gms.cast.framework.OptionsProvider
import com.google.android.gms.cast.framework.SessionProvider
import com.google.android.gms.cast.framework.media.CastMediaOptions

/**
 * Cast options provider for Chromecast integration
 */
class CastOptionsProvider : OptionsProvider {
    override fun getCastOptions(context: Context): CastOptions {
        // Configure Cast options
        return CastOptions.Builder()
            .setReceiverApplicationId(CastMediaOptions.DEFAULT_MEDIA_RECEIVER_APPLICATION_ID)
            .build()
    }

    override fun getAdditionalSessionProviders(context: Context): List<SessionProvider>? {
        return null
    }
}