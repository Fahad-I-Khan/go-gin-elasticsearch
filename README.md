# Blog Management System with Elasticsearch and PostgreSQL

This is a RESTful API application for managing blogs, built using **Gin** for routing, **GORM** for ORM, **PostgreSQL** for the database, and **Elasticsearch** for powerful text search capabilities. The application is containerized using Docker.

## Features

- Create, read, update, and delete blogs.
- Advanced search functionality with Elasticsearch, supporting filters and pagination.
- Seamless integration of PostgreSQL for data storage and Elasticsearch for efficient search.

## Getting Started

### Prerequisites

- Docker and Docker Compose
- PostgreSQL and Elasticsearch running as containers
- Go 1.20 or higher

## API Endpoints

- `POST /blogs` - Create a new blog.
- `GET /blogs/:id` - Retrieve a blog by its ID.
- `PUT /blogs/:id` - Update a blog.
- `DELETE /blogs/:id` - Delete a blog.
- `GET /blogs/search` - Search blogs using Elasticsearch.

## Elasticsearch Integration

### `initElasticsearch()`

- **Purpose**: This function initializes the Elasticsearch client. It uses a retry mechanism (up to 5 attempts) in case the initial connection to Elasticsearch fails.
- **Steps**:
  - The function tries to create a new Elasticsearch client using `elasticsearch.NewDefaultClient()`.
  - The `Ping()` function checks if the connection to Elasticsearch is alive.
  - If the connection fails, it waits 5 seconds and tries again up to 5 times.

Example Log Output:

```vbnet
Attempt 1: Failed to ping Elasticsearch: connection refused
Attempt 2: Connected to Elasticsearch.
```

## 1. Creating a Blog

`createBlog` function:

This function adds a new blog to both `PostgreSQL` and `Elasticsearch`.

**Example Blog Creation Flow**:

- **Incoming Request** (`POST /blogs`):

```json
{
    "title": "GoLang for Beginners",
    "content": "This is a beginner's guide to GoLang.",
    "author": "Jane Doe",
    "category": "Programming"
}
```

### Step-by-step Execution:

1. **Parsing JSON Input:**

- The **JSON** request is parsed and bound to a `Blog` struct using `c.ShouldBindJSON(&blog)`.
- This step populates the blog variable:

```go
blog := Blog{
    Title:     "GoLang for Beginners",
    Content:   "This is a beginner's guide to GoLang.",
    Author:    "Jane Doe",
    Category:  "Programming",
    CreatedAt: time.Now(),  // Let's assume this is "2024-12-29T10:00:00Z"
}
```
2. **Saving Blog to PostgreSQL:**

- The blog is then saved to **PostgreSQL** using `db.Create(&blog)`.
- It inserts a new record in the `blogs` table with values for `Title`, `Content`, `Author`, `Category`, and `CreatedAt`.

**SQL Equivalent:**
```sql
INSERT INTO blogs (title, content, author, category, created_at)
VALUES ('GoLang for Beginners', 'This is a beginner\'s guide to GoLang.', 'Jane Doe', 'Programming', '2024-12-29T10:00:00Z');
```
3. **Preparing Document for Elasticsearch:**

- Next, we prepare a **JSON document** to index in **Elasticsearch**:

```go
doc := map[string]interface{}{
    "title":     blog.Title,
    "content":   blog.Content,
    "author":    blog.Author,
    "category":  blog.Category,
    "createdAt": blog.CreatedAt.Format(time.RFC3339),
}
```
This creates a map with the same blog data.

**Resulting JSON Document**:

```json
{
    "title": "GoLang for Beginners",
    "content": "This is a beginner's guide to GoLang.",
    "author": "Jane Doe",
    "category": "Programming",
    "createdAt": "2024-12-29T10:00:00Z"
}
```

4.  **Indexing the Blog into Elasticsearch**:

- The `es.Index()` method is called to index this document into Elasticsearch:
```go
_, err := es.Index(
    "blogs",
    esutil.NewJSONReader(doc),
)
```
- Indexing Operation:
  - The document is indexed in the blogs index.
  - It automatically creates a unique document ID for the new blog.

**Elasticsearch Query** (simplified):
```json
POST /blogs/_doc
{
    "title": "GoLang for Beginners",
    "content": "This is a beginner's guide to GoLang.",
    "author": "Jane Doe",
    "category": "Programming",
    "createdAt": "2024-12-29T10:00:00Z"
}
```

5. **Final Response**:

- After successful creation, a **201 Created** response is returned to the client, with the blog details.

- **Response**:

```json
{
    "id": 1,
    "title": "GoLang for Beginners",
    "content": "This is a beginner's guide to GoLang.",
    "author": "Jane Doe",
    "category": "Programming",
    "createdAt": "2024-12-29T10:00:00Z"
}
```

## Updating a Blog

`updateBlog` function:

This function updates an existing blog in **PostgreSQL** and **Elasticsearch**.

**Example Blog Update Flow**:

- **Incoming Request** (`PUT /blogs/1`):

```json
{
    "title": "GoLang for Advanced Learners",
    "content": "This is a detailed guide for advanced GoLang developers.",
    "author": "Jane Doe",
    "category": "Programming"
}
```

### Step-by-step Execution:

1. **Fetching the Existing Blog**:

- We first fetch the existing blog from PostgreSQL using `db.First(&blog, id)`, where `id = 1`.

**Resulting** `blog` **Struct** (before update):

```go
blog := Blog{
    ID:        1,
    Title:     "GoLang for Beginners",
    Content:   "This is a beginner's guide to GoLang.",
    Author:    "Jane Doe",
    Category:  "Programming",
    CreatedAt: time.Date(2024, 12, 29, 10, 0, 0, 0, time.UTC),
}
```

2. **Parsing Updated Data**:

The updated blog data from the request body is parsed into the `updatedBlog` struct:

```go
updatedBlog := Blog{
    Title:     "GoLang for Advanced Learners",
    Content:   "This is a detailed guide for advanced GoLang developers.",
    Author:    "Jane Doe",
    Category:  "Programming",
}
```

3. **Deleting the Old Document from Elasticsearch**:

Before updating the blog in Elasticsearch, we delete the **old document** using `es.Delete()` with the `id` of the blog:

```go
_, err := es.Delete(
    "blogs",
    id,
    es.Delete.WithRefresh("true"),
)
```
- **Elasticsearch Deletion**:

The document with the ID `1` is removed from the `blogs` index.

**Elasticsearch Query**:

```json
DELETE /blogs/_doc/1
```

4. **Updating the Blog in PostgreSQL**:
- We update the blog in **PostgreSQL** using `db.Save(&blog)`:

```go
blog.Title = updatedBlog.Title
blog.Content = updatedBlog.Content
blog.Author = updatedBlog.Author
blog.Category = updatedBlog.Category
db.Save(&blog)
```

**SQL Equivalent**:
```sql
UPDATE blogs
SET title = 'GoLang for Advanced Learners',
    content = 'This is a detailed guide for advanced GoLang developers.',
    author = 'Jane Doe',
    category = 'Programming'
WHERE id = 1;
```

5. **Convert uint Blog ID to string for Elasticsearch**:

```go
// Convert blog.ID (uint) to string for Elasticsearch
idString := strconv.FormatUint(uint64(blog.ID), 10) // Convert uint to string
```
- **Conversion of** `uint` to `string`: The `blog.ID` in PostgreSQL is of type `uint` (unsigned integer). However, Elasticsearch expects the document ID to be a `string`.
- `strconv.FormatUint(uint64(blog.ID), 10)`:
  - `uint64(blog.ID)`: We first convert the `uint` to `uint64` (because `strconv.FormatUint` only works with `uint64`).
  - `10`: The second argument specifies the base (decimal, i.e., base 10). This converts the `uint` (e.g., `1`) into the string `"1"`.

6. **Reindex the Updated Blog in Elasticsearch**:

```go
// Reindex the updated blog in Elasticsearch
doc := map[string]interface{}{
    "title":     blog.Title,
    "content":   blog.Content,
    "author":    blog.Author,
    "category":  blog.Category,
    "createdAt": blog.CreatedAt.Format(time.RFC3339),
}

_, err = es.Index(
    "blogs",                          // The name of the Elasticsearch index
    esutil.NewJSONReader(doc),         // The updated blog document
    es.Index.WithDocumentID(idString), // Use the same ID as the PostgreSQL ID (converted to string)
    es.Index.WithRefresh("true"),      // Refresh the index after indexing
)
if err != nil {
    c.JSON(500, gin.H{"error": "Failed to index updated blog in Elasticsearch"})
    return
}
```

- `doc`: A map containing the blog fields (`title`, `content`, etc.), which are taken from the `blog` object and prepared for indexing in Elasticsearch.
- `es.Index(...)`: Sends the updated blog document to Elasticsearch.
- `"blogs"`: The index where blog documents are stored.
- `esutil.NewJSONReader(doc)`: Converts the `doc` map into a JSON reader for Elasticsearch.
- `es.Index.WithDocumentID(idString)`: Uses the blog's `idString` (converted from the `uint` ID) as the document ID in Elasticsearch, ensuring the same document is updated, not a new one.
- `es.Index.WithRefresh("true")`: Refreshes the index so the updated document is immediately searchable.

This ensures consistency between your PostgreSQL and Elasticsearch data, preventing duplicate blog entries.