# GitAI: AI-Powered Git Reviewer & Commit Generator

This project is an AI-powered Git reviewer and helper designed to automate commit message generation, review code modifications, and assist in developer workflows using the Gemini API.

## Client Testing

You can run the test client by providing your Gemini API key:

### Run Default Test (Oceans Query)
```bash
GEMINI_API_KEY="your_api_key_here" go run cmd/main.go
```

### Run Custom Prompt
```bash
GEMINI_API_KEY="your_api_key_here" go run cmd/main.go "What is the capital of France?"
```

### Run Git Diff Review & Auto Commit
You can pass the following flags to work with your current changes:

*   **Generate and print a suggested commit message**:
    ```bash
    go run cmd/main.go -commitmsg
    ```

*   **Generate a commit message and automatically commit the changes**:
    ```bash
    go run cmd/main.go -commit
    ```
