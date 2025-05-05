FROM python:3.11-slim-buster

WORKDIR /app

COPY Pipfile Pipfile.lock requirements.txt ./

RUN pip install --upgrade pip && \
    pip install -r requirements.txt

COPY . .

CMD ["python", "main.py"]
