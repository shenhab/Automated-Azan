FROM python:3.11-slim-buster

WORKDIR /app

COPY Pipfile Pipfile.lock ./

RUN pip install --upgrade pip && \
    pip install -r Pipfile.lock

COPY . .

CMD ["python", "main.py"]
