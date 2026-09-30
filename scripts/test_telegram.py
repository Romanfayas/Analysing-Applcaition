#!/usr/bin/env python3
"""
Test script for the Telegram Notification Service.

Usage:
  python3 scripts/test_telegram.py <CHAT_ID>

You can find your Chat ID by messaging @userinfobot on Telegram.
"""

import sys
import os
import requests
import json

# Provided Telegram Bot Token
BOT_TOKEN = "7727195753:AAHNNktMqAU0XmBS2H-5lthHTW-AsM5x25I"

def send_test_message(chat_id: str):
    url = f"https://api.telegram.org/bot{BOT_TOKEN}/sendMessage"
    
    payload = {
        "chat_id": chat_id,
        "text": "🟢 *Halal Equity Platform*\n\nThis is a test notification. Your notification system is successfully configured!",
        "parse_mode": "Markdown"
    }
    
    headers = {
        "Content-Type": "application/json"
    }
    
    print(f"Sending test message to Chat ID: {chat_id}...")
    response = requests.post(url, data=json.dumps(payload), headers=headers)
    
    if response.status_code == 200:
        print("✅ Message sent successfully!")
    else:
        print(f"❌ Failed to send message. Status: {response.status_code}")
        print(response.text)

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Error: Missing Chat ID.")
        print("Usage: python3 test_telegram.py <YOUR_CHAT_ID>")
        print("Find your Chat ID by messaging @userinfobot on Telegram.")
        sys.exit(1)
        
    chat_id = sys.argv[1]
    send_test_message(chat_id)
