package com.automatedazan.ui

import android.os.Bundle
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.mediarouter.media.MediaRouter
import com.automatedazan.R
import com.automatedazan.data.PreferenceManager
import com.automatedazan.databinding.ActivitySettingsBinding
import com.automatedazan.worker.PrayerTimesSchedulerWorker
import com.google.android.gms.cast.framework.CastContext

class SettingsActivity : AppCompatActivity() {

    private lateinit var binding: ActivitySettingsBinding
    private var mediaRouter: MediaRouter? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        binding = ActivitySettingsBinding.inflate(layoutInflater)
        setContentView(binding.root)
        
        // Set up toolbar with back button
        setSupportActionBar(binding.toolbar)
        supportActionBar?.setDisplayHomeAsUpEnabled(true)
        supportActionBar?.title = getString(R.string.settings)
        
        // Initialize the MediaRouter for Cast device discovery
        mediaRouter = MediaRouter.getInstance(this)
        
        // Initialize CastContext for discovery
        try {
            CastContext.getSharedInstance(this)
        } catch (e: Exception) {
            // Handle initialization error
        }
        
        // Load current settings
        loadSettings()
        
        // Set up event listeners
        setupEventListeners()
    }
    
    private fun loadSettings() {
        // Location Settings
        if (PreferenceManager.getPrayerLocation() == PreferenceManager.LOCATION_ICCI) {
            binding.radioIcci.isChecked = true
        } else {
            binding.radioNaas.isChecked = true
        }
        
        // Cast Device Settings
        binding.castDeviceEditText.setText(PreferenceManager.getCastDeviceName())
    }
    
    private fun setupEventListeners() {
        // Scan Devices Button
        binding.scanDevicesButton.setOnClickListener {
            // This would typically launch the Cast device discovery dialog
            // For simplicity, we're just using manual entry
            Toast.makeText(this, "Cast device discovery would open here", Toast.LENGTH_SHORT).show()
            
            // In a real implementation, you would:
            // 1. Create an MediaRouteSelector for Cast devices
            // 2. Show the MediaRouteChooserDialog
            // 3. Handle device selection
        }
        
        // Save Settings Button
        binding.saveSettingsButton.setOnClickListener {
            saveSettings()
        }
    }
    
    private fun saveSettings() {
        // Save Location Setting
        val location = if (binding.radioIcci.isChecked) 
            PreferenceManager.LOCATION_ICCI 
        else 
            PreferenceManager.LOCATION_NAAS
        PreferenceManager.setPrayerLocation(location)
        
        // Save Cast Device
        val castDeviceName = binding.castDeviceEditText.text.toString().trim()
        if (castDeviceName.isEmpty()) {
            Toast.makeText(this, "Please enter a cast device name", Toast.LENGTH_SHORT).show()
            return
        }
        PreferenceManager.setCastDeviceName(castDeviceName)
        
        // Schedule prayer times if settings are valid
        if (PreferenceManager.isSetupComplete()) {
            PrayerTimesSchedulerWorker.schedule(applicationContext)
            Toast.makeText(this, R.string.setup_complete, Toast.LENGTH_SHORT).show()
            finish()
        } else {
            Toast.makeText(this, R.string.setup_required, Toast.LENGTH_LONG).show()
        }
    }
    
    override fun onSupportNavigateUp(): Boolean {
        finish()
        return true
    }
}