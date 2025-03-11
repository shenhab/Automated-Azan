Here’s a **better, more structured, and professional** version of your GitHub **README.md**:

---

# **Automated Azan ⏰🕌**  
*A Python-based tool to automate Azan notifications using Google Home devices.*

---

## **📌 Features**
✅ Plays Azan automatically based on your configured location.  
✅ Supports Google Home speakers via `pychromecast`.  
✅ Customizable settings via `adahn.config`.  
✅ Runs continuously as a background service.  

---

## **🚀 Installation & Setup**
### **1️⃣ Prerequisites**
Before you begin, ensure you have the following installed:

- **Python 3** (Check [here](https://www.python.org/downloads/) if you don't have it)
- **Pipenv** for dependency management → [Installation Guide](https://pipenv.pypa.io/en/latest/)
- **Git CLI** for cloning the repository → [Download Git](https://git-scm.com/)

### **2️⃣ Installation Steps**
Run the following commands in your terminal:

```bash
# Clone the repository
git clone https://github.com/shenhab/Automated-Azan.git

# Navigate to the project directory
cd Automated-Azan

# Install dependencies using Pipenv
pipenv install
```

### **3️⃣ Configure the Application**
Edit the `adahn.config` file to set your **speakers group name** and **location**.  

Example:
```
[Settings]
speakers-group-name = MyGoogleSpeaker
location = Mosque-location [naas,icci]
```

### **4️⃣ Run the Application**
Once configured, start the **Automated Azan** script:

```bash
pipenv run python main.py
```

---

## **🔧 Troubleshooting**
- **Error: ModuleNotFoundError?**  
  Run `pipenv install` again inside the project folder.
- **No sound from Google Home?**  
  - Ensure your Google Home device is on the same network.
  - Check if the `speakers-group-name` in `adahn.config` matches your Google device.

---

## **💡 Future Improvements**
- Implement a web interface for easy configuration  
- Allow integration with more audio sources  

---

## **📜 License**
This project is licensed under the **MIT License**. Feel free to use and contribute!  

---

Let me know if you want to add more details or make it even better! 🚀🎯
